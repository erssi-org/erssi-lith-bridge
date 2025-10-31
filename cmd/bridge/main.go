package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"

	"erssi-lith-bridge/internal/bridge"

	"github.com/sirupsen/logrus"
)

var (
	erssiURL      = flag.String("erssi", "ws://localhost:9001", "erssi WebSocket URL")
	erssiPassword = flag.String("password", "", "erssi WebSocket password")
	listenAddr    = flag.String("listen", ":9000", "WeeChat protocol listen address")
	verbose       = flag.Bool("v", false, "Verbose logging")
	version       = "0.1.0"
)

func main() {
	flag.Parse()

	// Setup logger
	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	if *verbose {
		logger.SetLevel(logrus.DebugLevel)
	} else {
		logger.SetLevel(logrus.InfoLevel)
	}

	logger.Infof("erssi-Lith Bridge v%s", version)
	logger.Infof("erssi URL: %s", *erssiURL)
	logger.Infof("Listening on: %s", *listenAddr)

	// Create bridge
	b, err := bridge.New(bridge.Config{
		ErssiURL:      *erssiURL,
		ErssiPassword: *erssiPassword,
		ListenAddr:    *listenAddr,
		Logger:        logger,
	})
	if err != nil {
		logger.Fatalf("Failed to create bridge: %v", err)
	}

	// Start bridge
	if err := b.Start(); err != nil {
		logger.Fatalf("Failed to start bridge: %v", err)
	}

	// Wait for signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	logger.Info("Bridge running, press Ctrl+C to stop...")

	// Wait for signal or connection close
	select {
	case sig := <-sigChan:
		logger.Infof("Received signal %v, shutting down...", sig)
	case <-waitForDone(b):
		logger.Info("Connection closed")
	}

	// Stop bridge
	if err := b.Stop(); err != nil {
		logger.Errorf("Error stopping bridge: %v", err)
	}

	logger.Info("Bridge stopped, goodbye!")
}

func waitForDone(b *bridge.Bridge) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		b.Wait()
		close(done)
	}()
	return done
}
