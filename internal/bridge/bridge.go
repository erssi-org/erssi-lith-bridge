package bridge

import (
	"fmt"
	"sync"

	"erssi-lith-bridge/internal/erssi"
	"erssi-lith-bridge/internal/translator"
	"erssi-lith-bridge/internal/weechat"
	"erssi-lith-bridge/pkg/erssiproto"

	"github.com/sirupsen/logrus"
)

// Bridge connects erssi WebSocket to WeeChat protocol clients
type Bridge struct {
	erssiClient   *erssi.Client
	weechatServer *weechat.Server
	translator    *translator.Translator

	log *logrus.Entry

	// Synchronization
	mu      sync.RWMutex
	running bool
}

// Config holds bridge configuration
type Config struct {
	// erssi connection
	ErssiURL      string
	ErssiPassword string

	// WeeChat server
	ListenAddr string

	// Logging
	Logger *logrus.Logger
}

// New creates a new bridge instance
func New(cfg Config) (*Bridge, error) {
	logger := cfg.Logger
	if logger == nil {
		logger = logrus.New()
		logger.SetLevel(logrus.DebugLevel)
	}

	// Create erssi client
	erssiClient := erssi.NewClient(erssi.Config{
		URL:      cfg.ErssiURL,
		Password: cfg.ErssiPassword,
		Logger:   logger,
	})

	// Create WeeChat server
	weechatServer := weechat.NewServer(weechat.Config{
		Address: cfg.ListenAddr,
		Logger:  logger,
	})

	// Create translator
	trans := translator.NewTranslator(logger)

	b := &Bridge{
		erssiClient:   erssiClient,
		weechatServer: weechatServer,
		translator:    trans,
		log:           logger.WithField("component", "bridge"),
	}

	// Setup handlers
	b.setupHandlers()

	return b, nil
}

// setupHandlers configures event handlers
func (b *Bridge) setupHandlers() {
	// erssi client handlers
	b.erssiClient.OnMessage(b.handleErssiMessage)
	b.erssiClient.OnConnected(b.handleErssiConnected)
	b.erssiClient.OnDisconnect(b.handleErssiDisconnect)

	// WeeChat server handlers
	b.weechatServer.OnCommand(b.handleWeeChatCommand)
	b.weechatServer.OnClientConnected(b.handleWeeChatClientConnected)
	b.weechatServer.OnClientDisconnected(b.handleWeeChatClientDisconnected)
}

// Start starts the bridge
func (b *Bridge) Start() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.running {
		return fmt.Errorf("bridge already running")
	}

	b.log.Info("Starting bridge...")

	// Start WeeChat server
	if err := b.weechatServer.Start(); err != nil {
		return fmt.Errorf("failed to start WeeChat server: %w", err)
	}

	// Connect to erssi
	if err := b.erssiClient.Connect(); err != nil {
		b.weechatServer.Close()
		return fmt.Errorf("failed to connect to erssi: %w", err)
	}

	b.running = true
	b.log.Info("Bridge started successfully")

	return nil
}

// Stop stops the bridge
func (b *Bridge) Stop() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.running {
		return nil
	}

	b.log.Info("Stopping bridge...")

	// Close erssi connection
	if err := b.erssiClient.Close(); err != nil {
		b.log.Errorf("Error closing erssi client: %v", err)
	}

	// Close WeeChat server
	if err := b.weechatServer.Close(); err != nil {
		b.log.Errorf("Error closing WeeChat server: %v", err)
	}

	b.running = false
	b.log.Info("Bridge stopped")

	return nil
}

// Wait blocks until erssi connection is closed
func (b *Bridge) Wait() {
	b.erssiClient.Wait()
}

// erssi event handlers

func (b *Bridge) handleErssiMessage(msg *erssiproto.WebMessage) {
	b.log.Debugf("erssi message: type=%d from=%s target=%s", msg.Type, msg.Nick, msg.Target)

	// Translate message type
	switch msg.Type {
	case erssiproto.Message:
		// Convert to WeeChat line
		weechatMsg := b.translator.ErssiMessageToLine(msg)
		b.weechatServer.BroadcastMessage(weechatMsg)

	case erssiproto.StateDump:
		// Convert to buffer list
		weechatMsg := b.translator.ErssiToBufferList(msg)
		b.weechatServer.BroadcastMessage(weechatMsg)

	case erssiproto.Nicklist:
		// TODO: Parse nicklist from msg and convert
		// For now, create empty nicklist
		// weechatMsg := b.translator.ErssiNicklistToWeeChat(msg, nicks)
		// b.weechatServer.BroadcastMessage(weechatMsg)

	case erssiproto.ChannelJoin:
		// Handle channel join - update buffer list
		b.log.Debugf("Channel join: %s joined %s", msg.Nick, msg.Target)

	case erssiproto.ChannelPart:
		// Handle channel part
		b.log.Debugf("Channel part: %s left %s", msg.Nick, msg.Target)

	default:
		b.log.Debugf("Unhandled erssi message type: %d", msg.Type)
	}
}

func (b *Bridge) handleErssiConnected() {
	b.log.Info("Connected to erssi, requesting state dump...")

	// Request full state dump
	if err := b.erssiClient.RequestStateDump(); err != nil {
		b.log.Errorf("Failed to request state dump: %v", err)
	}
}

func (b *Bridge) handleErssiDisconnect(err error) {
	b.log.Errorf("Disconnected from erssi: %v", err)
	// TODO: Implement reconnection logic
}

// WeeChat event handlers

func (b *Bridge) handleWeeChatCommand(client *weechat.Client, cmd string, args []string) {
	b.log.Debugf("WeeChat command: %s args=%v", cmd, args)

	switch cmd {
	case "init":
		// Client authenticated, send initial data
		b.handleWeeChatInit(client, args)

	case "hdata":
		b.handleWeeChatHData(client, args)

	case "input":
		b.handleWeeChatInput(client, args)

	case "sync":
		b.handleWeeChatSync(client, args)

	case "nicklist":
		b.handleWeeChatNicklist(client, args)

	default:
		b.log.Warnf("Unhandled WeeChat command: %s", cmd)
	}
}

func (b *Bridge) handleWeeChatInit(client *weechat.Client, args []string) {
	b.log.Info("WeeChat client initialized, sending buffer list...")

	// Send buffer list
	msg := b.translator.GetAllBuffers()
	if err := client.SendMessage(msg); err != nil {
		b.log.Errorf("Failed to send buffer list: %v", err)
	}
}

func (b *Bridge) handleWeeChatHData(client *weechat.Client, args []string) {
	path, params, err := b.translator.ParseHDataCommand(args)
	if err != nil {
		b.log.Errorf("Invalid hdata command: %v", err)
		return
	}

	b.log.Debugf("HData request: path=%s params=%s", path, params)

	// Handle different hdata requests
	if path == "buffer:gui_buffers(*)" || path == "buffer:gui_buffers" {
		// Buffer list request
		msg := b.translator.GetAllBuffers()
		if err := client.SendMessage(msg); err != nil {
			b.log.Errorf("Failed to send buffers: %v", err)
		}
	} else if path == "hotlist:gui_hotlist(*)" {
		// Hotlist request - send empty for now
		// TODO: Implement hotlist
	} else {
		b.log.Warnf("Unhandled hdata path: %s", path)
	}
}

func (b *Bridge) handleWeeChatInput(client *weechat.Client, args []string) {
	bufferPtr, text, err := b.translator.ParseInputCommand(args)
	if err != nil {
		b.log.Errorf("Invalid input command: %v", err)
		return
	}

	b.log.Debugf("Input: buffer=%s text=%s", bufferPtr, text)

	// Convert to erssi command
	erssiMsg, err := b.translator.InputToErssiCommand(bufferPtr, text)
	if err != nil {
		b.log.Errorf("Failed to convert input: %v", err)
		return
	}

	// Send to erssi
	if err := b.erssiClient.SendMessage(erssiMsg); err != nil {
		b.log.Errorf("Failed to send message to erssi: %v", err)
	}
}

func (b *Bridge) handleWeeChatSync(client *weechat.Client, args []string) {
	b.log.Debug("Sync request - client wants updates")
	// Sync is automatic in our bridge - erssi pushes updates
	// Nothing to do here
}

func (b *Bridge) handleWeeChatNicklist(client *weechat.Client, args []string) {
	b.log.Debug("Nicklist request")
	// TODO: Request nicklist from erssi
}

func (b *Bridge) handleWeeChatClientConnected(client *weechat.Client) {
	b.log.Info("New WeeChat client connected")
}

func (b *Bridge) handleWeeChatClientDisconnected(client *weechat.Client) {
	b.log.Info("WeeChat client disconnected")
}
