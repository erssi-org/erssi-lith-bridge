package erssi

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"erssi-lith-bridge/pkg/erssiproto"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

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

	return &Client{
		url:      cfg.URL,
		password: cfg.Password,
		log:      logger.WithField("component", "erssi-client"),
		done:     make(chan struct{}),
	}
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
	c.log.Infof("Connecting to erssi at %s", c.url)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.Dial(c.url, nil)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	// Start read loop
	go c.readLoop()

	// Authenticate if password is set
	if c.password != "" {
		if err := c.authenticate(); err != nil {
			c.Close()
			return fmt.Errorf("authentication failed: %w", err)
		}
	}

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

		_, data, err := conn.ReadMessage()
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

		// Parse JSON message
		var msg erssiproto.WebMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			c.log.Errorf("Failed to parse message: %v", err)
			continue
		}

		c.log.Debugf("Received message type=%d from=%s target=%s", msg.Type, msg.Nick, msg.Target)

		// Call message handler
		c.mu.RLock()
		if c.onMessage != nil {
			go c.onMessage(&msg)
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

	c.log.Debugf("Sending message type=%d", msg.Type)

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
		Type: 16, // TODO: Define proper constant for state dump request
	}

	return c.SendMessage(msg)
}

// RequestNicklist requests nicklist for a channel
func (c *Client) RequestNicklist(serverTag, channel string) error {
	msg := &erssiproto.WebMessage{
		Type:      9, // Nicklist
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
