package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"

	"erssi-lith-bridge/internal/bridge"

	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
)

var (
	erssiURL      *string
	erssiPassword *string
	listenAddr    *string
	verbose       *bool
	version       = "0.1.0"
)

func main() {
	// Load .env file if it exists (ignore error if not found)
	_ = godotenv.Load()

	// Get defaults from environment variables or use hardcoded defaults
	defaultErssiURL := getEnv("ERSSI_URL", "ws://localhost:9001")
	defaultPassword := getEnv("ERSSI_PASSWORD", "")
	defaultListen := getEnv("LISTEN_ADDR", ":9000")
	defaultVerbose := getEnv("VERBOSE", "false") == "true"

	// Define flags (these override environment variables)
	erssiURL = flag.String("erssi", defaultErssiURL, "erssi WebSocket URL (env: ERSSI_URL)")
	erssiPassword = flag.String("password", defaultPassword, "erssi WebSocket password (env: ERSSI_PASSWORD)")
	listenAddr = flag.String("listen", defaultListen, "WeeChat protocol listen address (env: LISTEN_ADDR)")
	verbose = flag.Bool("v", defaultVerbose, "Verbose logging (env: VERBOSE)")

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

// getEnv gets an environment variable with a fallback default value
func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
