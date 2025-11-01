package bridge

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
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
	mu                  sync.RWMutex
	running             bool
	inStateDump         bool   // Track if we're processing state_dump sequence
	stateDumpServer     string
	stateDumpRequested  bool   // Track if we already requested state dump from erssi
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
	b.log.Debugf("erssi message: type=%s from=%s target=%s", msg.Type, msg.Nick, msg.Target)

	// Translate message type
	switch msg.Type {
	case erssiproto.Message:
		// Convert IRC message to WeeChat line
		weechatMsg := b.translator.ErssiMessageToLine(msg)
		b.weechatServer.BroadcastMessage(weechatMsg)

	case erssiproto.StateDump:
		// state_dump marks the start of a server's state - create server buffer
		b.mu.Lock()
		b.inStateDump = true
		b.stateDumpServer = msg.ServerTag
		b.mu.Unlock()

		b.log.Infof("State dump started for server: %s", msg.ServerTag)

		// Create server buffer (network buffer)
		b.translator.EnsureServerBuffer(msg.ServerTag)
		b.log.Debugf("Created server buffer for: %s", msg.ServerTag)

		// Following channel_join messages will create channel buffers

	case erssiproto.Nicklist:
		// Parse nicklist from msg.Text (JSON array)
		b.handleNicklist(msg)

	case erssiproto.ChannelJoin:
		// Handle channel join
		b.handleChannelJoin(msg)

	case erssiproto.ChannelPart:
		// Handle channel part
		b.handleChannelPart(msg)

	case erssiproto.UserQuit:
		// Handle user quit
		b.handleUserQuit(msg)

	case erssiproto.Topic:
		// Handle topic change
		b.handleTopic(msg)

	case erssiproto.ActivityUpdate:
		// Handle activity update
		b.handleActivityUpdate(msg)

	default:
		b.log.Debugf("Unhandled erssi message type: %s", msg.Type)
	}
}

func (b *Bridge) handleErssiConnected() {
	b.log.Info("Connected to erssi, waiting for Lith clients...")
	// DON'T request state_dump here - wait until Lith connects and asks for buffers
}

func (b *Bridge) handleErssiDisconnect(err error) {
	b.log.Errorf("Disconnected from erssi: %v", err)
	// TODO: Implement reconnection logic
}

// Specific message type handlers

func (b *Bridge) handleNicklist(msg *erssiproto.WebMessage) {
	// Parse nicklist from msg.Text (JSON array)
	if msg.Text == "" {
		b.log.Warn("Nicklist message has empty text")
		return
	}

	var nicks []erssiproto.NickInfo
	if err := json.Unmarshal([]byte(msg.Text), &nicks); err != nil {
		b.log.Errorf("Failed to parse nicklist JSON: %v", err)
		return
	}

	b.log.Debugf("Received nicklist for %s.%s with %d users", msg.ServerTag, msg.Target, len(nicks))

	// Convert to WeeChat format and broadcast
	weechatMsg := b.translator.ErssiNicklistToWeeChat(msg, nicks)
	b.weechatServer.BroadcastMessage(weechatMsg)

	// Check if we're in state dump - nicklist is the last message per channel
	b.mu.RLock()
	inStateDump := b.inStateDump
	b.mu.RUnlock()

	if inStateDump {
		// During state dump, buffers are sent via handleBufferInitialization response
		// No need to broadcast _buffer_opened here
		b.log.Debug("Nicklist received during state dump")
	}
}

func (b *Bridge) handleChannelJoin(msg *erssiproto.WebMessage) {
	b.mu.RLock()
	inStateDump := b.inStateDump
	b.mu.RUnlock()

	if inStateDump {
		// During state dump - just ensure buffer exists (will be created by translator)
		b.log.Debugf("State dump: channel %s on %s", msg.Target, msg.ServerTag)
		// Create buffer via translator (it's idempotent)
		b.translator.EnsureBuffer(msg.ServerTag, msg.Target)
		return
	}

	// Real-time join event
	b.log.Debugf("Channel join: %s joined %s on %s", msg.Nick, msg.Target, msg.ServerTag)

	// Create a system message line for the join event
	joinMsg := &erssiproto.WebMessage{
		Type:      erssiproto.Message,
		ServerTag: msg.ServerTag,
		Target:    msg.Target,
		Nick:      "--",
		Text:      fmt.Sprintf("%s has joined %s", msg.Nick, msg.Target),
		Timestamp: msg.Timestamp,
	}

	weechatMsg := b.translator.ErssiMessageToLine(joinMsg)
	b.weechatServer.BroadcastMessage(weechatMsg)

	// Request updated nicklist for this channel
	if err := b.erssiClient.RequestNicklist(msg.ServerTag, msg.Target); err != nil {
		b.log.Errorf("Failed to request nicklist: %v", err)
	}
}

func (b *Bridge) handleChannelPart(msg *erssiproto.WebMessage) {
	b.log.Debugf("Channel part: %s left %s on %s", msg.Nick, msg.Target, msg.ServerTag)

	// Create a system message line for the part event
	partText := fmt.Sprintf("%s has left %s", msg.Nick, msg.Target)
	if msg.Text != "" {
		partText = fmt.Sprintf("%s has left %s (%s)", msg.Nick, msg.Target, msg.Text)
	}

	partMsg := &erssiproto.WebMessage{
		Type:      erssiproto.Message,
		ServerTag: msg.ServerTag,
		Target:    msg.Target,
		Nick:      "--",
		Text:      partText,
		Timestamp: msg.Timestamp,
	}

	weechatMsg := b.translator.ErssiMessageToLine(partMsg)
	b.weechatServer.BroadcastMessage(weechatMsg)

	// Request updated nicklist for this channel
	if err := b.erssiClient.RequestNicklist(msg.ServerTag, msg.Target); err != nil {
		b.log.Errorf("Failed to request nicklist: %v", err)
	}
}

func (b *Bridge) handleUserQuit(msg *erssiproto.WebMessage) {
	b.log.Debugf("User quit: %s quit from %s", msg.Nick, msg.ServerTag)

	// Create a system message line for the quit event
	quitText := fmt.Sprintf("%s has quit", msg.Nick)
	if msg.Text != "" {
		quitText = fmt.Sprintf("%s has quit (%s)", msg.Nick, msg.Text)
	}

	// If target is specified, send to that buffer
	if msg.Target != "" {
		quitMsg := &erssiproto.WebMessage{
			Type:      erssiproto.Message,
			ServerTag: msg.ServerTag,
			Target:    msg.Target,
			Nick:      "--",
			Text:      quitText,
			Timestamp: msg.Timestamp,
		}

		weechatMsg := b.translator.ErssiMessageToLine(quitMsg)
		b.weechatServer.BroadcastMessage(weechatMsg)
	}
}

func (b *Bridge) handleTopic(msg *erssiproto.WebMessage) {
	b.log.Debugf("Topic change: %s on %s.%s", msg.Text, msg.ServerTag, msg.Target)

	// Create a system message line for the topic change
	topicText := fmt.Sprintf("%s has changed topic to: %s", msg.Nick, msg.Text)
	if msg.Nick == "" {
		topicText = fmt.Sprintf("Topic: %s", msg.Text)
	}

	topicMsg := &erssiproto.WebMessage{
		Type:      erssiproto.Message,
		ServerTag: msg.ServerTag,
		Target:    msg.Target,
		Nick:      "--",
		Text:      topicText,
		Timestamp: msg.Timestamp,
	}

	weechatMsg := b.translator.ErssiMessageToLine(topicMsg)
	b.weechatServer.BroadcastMessage(weechatMsg)

	// Also broadcast buffer update to refresh topic for this specific buffer
	bufferUpdate := b.translator.GetBufferOpenedEvent(msg.ServerTag, msg.Target)
	b.weechatServer.BroadcastMessage(bufferUpdate)
}

func (b *Bridge) handleActivityUpdate(msg *erssiproto.WebMessage) {
	b.log.Debugf("Activity update for %s.%s", msg.ServerTag, msg.Target)
	// Activity updates are handled implicitly when messages arrive
	// WeeChat clients will show unread indicators based on new lines
	// Nothing specific to do here
}

// WeeChat event handlers

func (b *Bridge) handleWeeChatCommand(client *weechat.Client, msgID string, cmd string, args []string) {
	b.log.Debugf("WeeChat command: %s msgID=%s args=%v", cmd, msgID, args)

	switch cmd {
	case "init":
		// Client authenticated, send initial data
		b.handleWeeChatInit(client, msgID, args)

	case "hdata":
		b.handleWeeChatHData(client, msgID, args)

	case "input":
		b.handleWeeChatInput(client, msgID, args)

	case "sync":
		b.handleWeeChatSync(client, msgID, args)

	case "nicklist":
		b.handleWeeChatNicklist(client, msgID, args)

	default:
		b.log.Warnf("Unhandled WeeChat command: %s", cmd)
	}
}

func (b *Bridge) handleWeeChatInit(client *weechat.Client, msgID string, args []string) {
	b.log.Info("WeeChat client initialized")

	b.mu.Lock()
	needsStateDump := !b.stateDumpRequested
	if needsStateDump {
		b.stateDumpRequested = true
	}
	b.mu.Unlock()

	// Request state dump from erssi ONLY on first Lith connection
	if needsStateDump {
		b.log.Info("First client connection - requesting state from erssi...")
		if err := b.erssiClient.RequestStateDump(); err != nil {
			b.log.Errorf("Failed to request state dump: %v", err)
		}
	} else {
		b.log.Info("Subsequent client connection - using cached buffers")
	}

	// DO NOT send buffers here - Lith will request them via hdata buffer:gui_buffers(*)
	// Sending buffers before Lith is ready causes them to be ignored
}

func (b *Bridge) handleWeeChatHData(client *weechat.Client, msgID string, args []string) {
	path, params, err := b.translator.ParseHDataCommand(args)
	if err != nil {
		b.log.Errorf("Invalid hdata command: %v", err)
		return
	}

	b.log.Debugf("HData request: path=%s params=%s msgID=%s", path, params, msgID)

	// Handle different hdata requests
	if path == "buffer:gui_buffers(*)" || path == "buffer:gui_buffers" {
		// Buffer list request
		msg := b.translator.GetAllBuffers(msgID)
		b.log.Debugf("Sending buffer list response with ID '%s' (count: %d buffers)", msgID, len(b.translator.GetBufferList()))
		if err := client.SendMessage(msg); err != nil {
			b.log.Errorf("Failed to send buffers: %v", err)
		} else {
			b.log.Debug("Buffer list sent successfully")
		}
	} else if strings.Contains(path, "lines") {
		// Line history request - format: buffer:0x123/lines/last_line(-50)
		b.handleLineRequest(client, msgID, path, params)
	} else if path == "hotlist:gui_hotlist(*)" {
		// Hotlist request - send empty hotlist
		msg := b.translator.GetEmptyHotlist(msgID)
		b.log.Debugf("Sending empty hotlist response with ID '%s'", msgID)
		if err := client.SendMessage(msg); err != nil {
			b.log.Errorf("Failed to send hotlist: %v", err)
		} else {
			b.log.Debug("Hotlist sent successfully")
		}
	} else {
		b.log.Warnf("Unhandled hdata path: %s", path)
	}
}

func (b *Bridge) handleWeeChatInput(client *weechat.Client, msgID string, args []string) {
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

func (b *Bridge) handleWeeChatSync(client *weechat.Client, msgID string, args []string) {
	b.log.Debug("Sync request - client wants updates")
	// Sync is automatic in our bridge - erssi pushes updates
	// Nothing to do here
}

func (b *Bridge) handleWeeChatNicklist(client *weechat.Client, msgID string, args []string) {
	b.log.Debugf("Nicklist request: args=%v", args)

	// Parse nicklist request - format varies, but typically includes buffer pointer
	if len(args) == 0 {
		b.log.Warn("Nicklist request with no args")
		return
	}

	// Extract buffer pointer and request nicklist from erssi
	bufferPtr := args[0]
	serverTag, target := b.translator.GetBufferInfo(bufferPtr)

	if serverTag != "" && target != "" {
		b.log.Debugf("Requesting nicklist for %s.%s", serverTag, target)
		if err := b.erssiClient.RequestNicklist(serverTag, target); err != nil {
			b.log.Errorf("Failed to request nicklist: %v", err)
		}
	}
}

func (b *Bridge) handleLineRequest(client *weechat.Client, msgID string, path, params string) {
	// Parse buffer pointer from path
	// Format: buffer:0x123/lines/last_line(-50)
	re := regexp.MustCompile(`buffer:(0x[0-9a-f]+)`)
	matches := re.FindStringSubmatch(path)

	if len(matches) < 2 {
		b.log.Warnf("Could not parse buffer pointer from path: %s", path)
		return
	}

	bufferPtr := matches[1]

	// Parse line count from params (e.g., "(-50)")
	count := 50 // default
	if params != "" {
		re2 := regexp.MustCompile(`\((-?\d+)\)`)
		matches2 := re2.FindStringSubmatch(params)
		if len(matches2) >= 2 {
			if n, err := strconv.Atoi(matches2[1]); err == nil {
				if n < 0 {
					count = -n
				} else {
					count = n
				}
			}
		}
	}

	b.log.Debugf("Line request for buffer %s, count=%d, msgID=%s", bufferPtr, count, msgID)

	// Get lines from translator
	msg := b.translator.GetBufferLines(bufferPtr, count, msgID)
	if err := client.SendMessage(msg); err != nil {
		b.log.Errorf("Failed to send lines: %v", err)
	}
}

func (b *Bridge) handleWeeChatClientConnected(client *weechat.Client) {
	b.log.Info("New WeeChat client connected")
}

func (b *Bridge) handleWeeChatClientDisconnected(client *weechat.Client) {
	b.log.Info("WeeChat client disconnected")
}
