package weechat

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"strings"
	"sync"

	"erssi-lith-bridge/pkg/weechatproto"

	"github.com/sirupsen/logrus"
)

// Server implements WeeChat relay protocol server
type Server struct {
	addr     string
	listener net.Listener
	log      *logrus.Entry

	// Client management
	clients   map[*Client]*Client
	clientsMu sync.RWMutex

	// Message handlers
	onCommand    func(*Client, string, string, []string) // client, msgID, command, args
	onClientConn func(*Client)
	onClientDisc func(*Client)

	done chan struct{}
}

// Config holds server configuration
type Config struct {
	Address string
	Logger  *logrus.Logger
}

// Client represents a connected Lith client
type Client struct {
	conn   net.Conn
	server *Server
	log    *logrus.Entry

	// Session state
	authenticated bool
	nonce         string

	// Writer for sending messages
	encoder *weechatproto.Encoder
	mu      sync.Mutex
}

// NewServer creates a new WeeChat protocol server
func NewServer(cfg Config) *Server {
	logger := cfg.Logger
	if logger == nil {
		logger = logrus.New()
	}

	return &Server{
		addr:    cfg.Address,
		log:     logger.WithField("component", "weechat-server"),
		clients: make(map[*Client]*Client),
		done:    make(chan struct{}),
	}
}

// OnCommand sets the command handler
func (s *Server) OnCommand(handler func(*Client, string, string, []string)) {
	s.onCommand = handler
}

// OnClientConnected sets the client connected handler
func (s *Server) OnClientConnected(handler func(*Client)) {
	s.onClientConn = handler
}

// OnClientDisconnected sets the client disconnected handler
func (s *Server) OnClientDisconnected(handler func(*Client)) {
	s.onClientDisc = handler
}

// Start starts the server
func (s *Server) Start() error {
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	s.listener = listener
	s.log.Infof("WeeChat protocol server listening on %s", s.addr)

	go s.acceptLoop()

	return nil
}

// acceptLoop accepts new client connections
func (s *Server) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.done:
				return
			default:
				s.log.Errorf("Accept error: %v", err)
				continue
			}
		}

		s.log.Infof("New client connected from %s", conn.RemoteAddr())

		client := &Client{
			conn:    conn,
			server:  s,
			log:     s.log.WithField("client", conn.RemoteAddr().String()),
			encoder: weechatproto.NewEncoder(conn),
		}

		s.clientsMu.Lock()
		s.clients[client] = client
		s.clientsMu.Unlock()

		// Notify about new client
		if s.onClientConn != nil {
			go s.onClientConn(client)
		}

		go s.handleClient(client)
	}
}

// handleClient handles a single client connection
func (s *Server) handleClient(client *Client) {
	defer func() {
		client.conn.Close()

		s.clientsMu.Lock()
		delete(s.clients, client)
		s.clientsMu.Unlock()

		client.log.Info("Client disconnected")

		// Notify about disconnection
		if s.onClientDisc != nil {
			go s.onClientDisc(client)
		}
	}()

	scanner := bufio.NewScanner(client.conn)
	for scanner.Scan() {
		line := scanner.Text()
		client.log.Debugf("Received command: %s", line)

		if err := s.handleCommand(client, line); err != nil {
			client.log.Errorf("Command error: %v", err)
			return
		}
	}

	if err := scanner.Err(); err != nil {
		client.log.Errorf("Scanner error: %v", err)
	}
}

// handleCommand parses and handles a WeeChat command
func (s *Server) handleCommand(client *Client, line string) error {
	// Parse command: (id) command arguments
	var msgID string
	var cmd string
	var args []string

	// Check for message ID
	if strings.HasPrefix(line, "(") {
		endIdx := strings.Index(line, ")")
		if endIdx == -1 {
			return fmt.Errorf("malformed message ID")
		}
		msgID = line[1:endIdx]
		line = strings.TrimSpace(line[endIdx+1:])
	}

	// Parse command and arguments
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return nil // Empty command
	}

	cmd = parts[0]
	if len(parts) > 1 {
		args = parts[1:]
	}

	client.log.Debugf("Command: %s, ID: %s, Args: %v", cmd, msgID, args)

	// Handle command
	switch cmd {
	case "handshake":
		return s.handleHandshake(client, msgID, args)
	case "init":
		return s.handleInit(client, msgID, args)
	case "hdata":
		return s.handleHData(client, msgID, args)
	case "input":
		return s.handleInput(client, msgID, args)
	case "sync":
		return s.handleSync(client, msgID, args)
	case "desync":
		return s.handleDesync(client, msgID, args)
	case "nicklist":
		return s.handleNicklist(client, msgID, args)
	case "quit":
		return fmt.Errorf("client requested quit")
	default:
		client.log.Warnf("Unknown command: %s", cmd)
	}

	return nil
}

// handleHandshake handles the handshake command
func (s *Server) handleHandshake(client *Client, msgID string, args []string) error {
	// Generate nonce
	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return err
	}
	client.nonce = hex.EncodeToString(nonceBytes)

	// Send handshake response
	msg := weechatproto.CreateHandshakeResponse(msgID, "plain", client.nonce)
	return client.SendMessage(msg)
}

// handleInit handles authentication
func (s *Server) handleInit(client *Client, msgID string, args []string) error {
	// TODO: Verify password
	// For now, accept all connections
	client.authenticated = true

	client.log.Info("Client authenticated")

	// Call command handler to trigger initial state sync
	if s.onCommand != nil {
		go s.onCommand(client, msgID, "init", args)
	}

	return nil
}

// handleHData handles hdata requests
func (s *Server) handleHData(client *Client, msgID string, args []string) error {
	if !client.authenticated {
		return fmt.Errorf("not authenticated")
	}

	// Forward to command handler
	if s.onCommand != nil {
		go s.onCommand(client, msgID, "hdata", args)
	}

	return nil
}

// handleInput handles input (send message) command
func (s *Server) handleInput(client *Client, msgID string, args []string) error {
	if !client.authenticated {
		return fmt.Errorf("not authenticated")
	}

	// Forward to command handler
	if s.onCommand != nil {
		go s.onCommand(client, msgID, "input", args)
	}

	return nil
}

// handleSync handles sync command
func (s *Server) handleSync(client *Client, msgID string, args []string) error {
	if !client.authenticated {
		return fmt.Errorf("not authenticated")
	}

	// Forward to command handler
	if s.onCommand != nil {
		go s.onCommand(client, msgID, "sync", args)
	}

	return nil
}

// handleDesync handles desync command
func (s *Server) handleDesync(client *Client, msgID string, args []string) error {
	if !client.authenticated {
		return fmt.Errorf("not authenticated")
	}

	// Forward to command handler
	if s.onCommand != nil {
		go s.onCommand(client, msgID, "desync", args)
	}

	return nil
}

// handleNicklist handles nicklist request
func (s *Server) handleNicklist(client *Client, msgID string, args []string) error {
	if !client.authenticated {
		return fmt.Errorf("not authenticated")
	}

	// Forward to command handler
	if s.onCommand != nil {
		go s.onCommand(client, msgID, "nicklist", args)
	}

	return nil
}

// SendMessage sends a message to the client
func (c *Client) SendMessage(msg *weechatproto.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.encoder.EncodeMessage(msg)
}

// BroadcastMessage sends a message to all connected clients
func (s *Server) BroadcastMessage(msg *weechatproto.Message) {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()

	for _, client := range s.clients {
		if client.authenticated {
			if err := client.SendMessage(msg); err != nil {
				client.log.Errorf("Failed to send message: %v", err)
			}
		}
	}
}

// Close closes the server
func (s *Server) Close() error {
	close(s.done)

	if s.listener != nil {
		return s.listener.Close()
	}

	return nil
}
