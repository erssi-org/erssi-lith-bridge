package erssi

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"erssi-lith-bridge/pkg/erssiproto"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Client represents a connection to erssi fe-web WebSocket server
type Client struct {
	url      string
	password string
	conn     *websocket.Conn
	mu       sync.RWMutex

	// Message handlers
	onMessage    func(*erssiproto.WebMessage)
	onConnected  func()
	onDisconnect func(error)

	// Internal state
	authenticated bool
	encryptionKey []byte // AES-256-GCM key
	log           *logrus.Entry
	done          chan struct{}
}

// Config holds configuration for erssi client
type Config struct {
	URL      string
	Password string
	Logger   *logrus.Logger
}

// NewClient creates a new erssi WebSocket client
func NewClient(cfg Config) *Client {
	logger := cfg.Logger
	if logger == nil {
		logger = logrus.New()
	}

	client := &Client{
		url:      cfg.URL,
		password: cfg.Password,
		log:      logger.WithField("component", "erssi-client"),
		done:     make(chan struct{}),
	}

	// Derive encryption key from password
	if cfg.Password != "" {
		client.encryptionKey = deriveKey(cfg.Password)
		client.log.Debug("Encryption key derived from password")
	}

	return client
}

// OnMessage sets the message handler
func (c *Client) OnMessage(handler func(*erssiproto.WebMessage)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onMessage = handler
}

// OnConnected sets the connected handler
func (c *Client) OnConnected(handler func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onConnected = handler
}

// OnDisconnect sets the disconnect handler
func (c *Client) OnDisconnect(handler func(error)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onDisconnect = handler
}

// Connect establishes connection to erssi WebSocket server
func (c *Client) Connect() error {
	// erssi requires password in query parameter: /?password=xxx
	urlWithPassword := c.url
	if c.password != "" {
		separator := "?"
		if strings.Contains(c.url, "?") {
			separator = "&"
		}
		urlWithPassword = fmt.Sprintf("%s%spassword=%s", c.url, separator, c.password)
	}

	c.log.Infof("Connecting to erssi at %s", c.url)
	c.log.Debugf("Full WebSocket URL with password: %s", urlWithPassword)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // erssi uses self-signed certs
		},
	}

	conn, resp, err := dialer.Dial(urlWithPassword, nil)
	if err != nil {
		if resp != nil {
			c.log.Errorf("HTTP Response Status: %s", resp.Status)
			c.log.Errorf("HTTP Response Headers: %v", resp.Header)
		}
		return fmt.Errorf("failed to connect: %w", err)
	}
	if resp != nil {
		c.log.Debugf("WebSocket handshake successful, status: %s", resp.Status)
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	// Start read loop
	go c.readLoop()

	// Password is already in URL query param, no separate auth needed
	c.authenticated = true
	c.log.Info("Connected to erssi")

	// Call connected handler
	c.mu.RLock()
	if c.onConnected != nil {
		go c.onConnected()
	}
	c.mu.RUnlock()

	return nil
}

// authenticate sends authentication to erssi
func (c *Client) authenticate() error {
	c.log.Debug("Authenticating...")

	auth := map[string]interface{}{
		"type":     "auth",
		"password": c.password,
	}

	data, err := json.Marshal(auth)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return err
	}

	// TODO: Wait for AUTH_OK response
	c.authenticated = true

	return nil
}

// readLoop continuously reads messages from WebSocket
func (c *Client) readLoop() {
	defer func() {
		c.log.Info("Read loop stopped")
		close(c.done)
	}()

	for {
		c.mu.RLock()
		conn := c.conn
		c.mu.RUnlock()

		if conn == nil {
			return
		}

		messageType, data, err := conn.ReadMessage()
		if err != nil {
			c.log.Errorf("Read error: %v", err)

			// Call disconnect handler
			c.mu.RLock()
			if c.onDisconnect != nil {
				go c.onDisconnect(err)
			}
			c.mu.RUnlock()

			return
		}

		// erssi sends binary frames for encrypted data
		if messageType == websocket.BinaryMessage && c.encryptionKey != nil {
			// Decrypt message
			decrypted, err := decryptMessage(data, c.encryptionKey)
			if err != nil {
				c.log.Errorf("Failed to decrypt message: %v", err)
				continue
			}
			data = decrypted
		}

		// Log raw JSON after decryption
		c.log.Debugf("Raw JSON received: %s", string(data))

		// Parse JSON message
		var msg erssiproto.WebMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			c.log.Errorf("Failed to parse message: %v", err)
			c.log.Debugf("Raw data (first 100 bytes): %q", string(data[:min(100, len(data))]))
			continue
		}

		// Log parsed message structure
		c.log.Debugf("Parsed message: type=%s, server_tag=%s, target=%s, nick=%s, text=%s, server=%s",
			msg.Type, msg.ServerTag, msg.Target, msg.Nick, msg.Text, msg.Server)

		c.log.Debugf("Received message type=%s from=%s target=%s", msg.Type, msg.Nick, msg.Target)

		// Call message handler
		c.mu.RLock()
		if c.onMessage != nil {
			// IMPORTANT: Create a copy of the message to avoid race conditions
			// The msg variable is reused in the loop, so we must copy it before
			// passing to the goroutine
			msgCopy := msg
			go c.onMessage(&msgCopy)
		}
		c.mu.RUnlock()
	}
}

// SendMessage sends a message to erssi
func (c *Client) SendMessage(msg *erssiproto.WebMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	c.log.Debugf("Sending message type=%s", msg.Type)

	if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	return nil
}

// SendCommand sends a command to erssi
func (c *Client) SendCommand(serverTag, target, text string) error {
	msg := &erssiproto.WebMessage{
		Type:      erssiproto.Message, // TODO: Use proper command type
		ServerTag: serverTag,
		Target:    target,
		Text:      text,
	}

	return c.SendMessage(msg)
}

// RequestStateDump requests full state dump from erssi
func (c *Client) RequestStateDump() error {
	msg := &erssiproto.WebMessage{
		Type:   erssiproto.SyncServer,
		Server: "*", // Request all servers
	}

	return c.SendMessage(msg)
}

// RequestNicklist requests nicklist for a channel
func (c *Client) RequestNicklist(serverTag, channel string) error {
	msg := &erssiproto.WebMessage{
		Type:      erssiproto.Nicklist,
		ServerTag: serverTag,
		Target:    channel,
	}

	return c.SendMessage(msg)
}

// Close closes the connection
func (c *Client) Close() error {
	c.log.Info("Closing connection")

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return nil
	}

	// Send close message
	err := c.conn.WriteMessage(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
	)

	// Close the connection
	if closeErr := c.conn.Close(); closeErr != nil && err == nil {
		err = closeErr
	}

	c.conn = nil

	return err
}

// Wait blocks until connection is closed
func (c *Client) Wait() {
	<-c.done
}
