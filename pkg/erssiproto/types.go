package erssiproto

import "time"

// MessageType represents erssi WebSocket message types (from fe-web.h)
type MessageType int

const (
	AuthOK MessageType = iota + 1
	Message
	ServerStatus
	ChannelJoin
	ChannelPart
	ChannelKick
	UserQuit
	Topic
	ChannelMode
	Nicklist
	NicklistUpdate
	NickChange
	UserMode
	Away
	Whois
	ChannelList
	StateDump
	Error
	Pong
	QueryOpened
	QueryClosed
	ActivityUpdate
	MarkRead
	NetworkList
	NetworkListResponse
	ServerList
	ServerListResponse
	NetworkAdd
	NetworkRemove
	ServerAdd
	ServerRemove
	CommandResult
)

// WebMessage represents a message from/to erssi fe-web
type WebMessage struct {
	ID             string                 `json:"id,omitempty"`
	Type           MessageType            `json:"type"`
	ServerTag      string                 `json:"server_tag,omitempty"`
	Target         string                 `json:"target,omitempty"`
	Nick           string                 `json:"nick,omitempty"`
	Text           string                 `json:"text,omitempty"`
	Level          int                    `json:"level,omitempty"`
	Timestamp      int64                  `json:"timestamp,omitempty"`
	IsOwn          bool                   `json:"is_own,omitempty"`
	IsHighlight    bool                   `json:"is_highlight,omitempty"`
	ExtraData      map[string]interface{} `json:"extra_data,omitempty"`
	ResponseTo     string                 `json:"response_to,omitempty"`
}

// NickInfo represents a user in a channel nicklist
type NickInfo struct {
	Nick   string `json:"nick"`
	Prefix string `json:"prefix,omitempty"`
	Mode   string `json:"mode,omitempty"`
	Host   string `json:"host,omitempty"`
}

// ChannelInfo represents channel metadata
type ChannelInfo struct {
	Name      string      `json:"name"`
	Topic     string      `json:"topic,omitempty"`
	Mode      string      `json:"mode,omitempty"`
	UserCount int         `json:"user_count,omitempty"`
	Nicks     []NickInfo  `json:"nicks,omitempty"`
}

// ServerInfo represents IRC server status
type ServerInfo struct {
	Tag       string    `json:"tag"`
	Address   string    `json:"address"`
	Port      int       `json:"port"`
	Connected bool      `json:"connected"`
	Nick      string    `json:"nick,omitempty"`
	Channels  []string  `json:"channels,omitempty"`
}

// AuthRequest represents authentication to erssi
type AuthRequest struct {
	Type     MessageType `json:"type"` // Should be auth request type
	Password string      `json:"password,omitempty"`
	Token    string      `json:"token,omitempty"`
}

// CommandRequest represents a command sent to erssi
type CommandRequest struct {
	Type      MessageType `json:"type"`
	ServerTag string      `json:"server_tag,omitempty"`
	Target    string      `json:"target,omitempty"`
	Command   string      `json:"command,omitempty"`
	Text      string      `json:"text,omitempty"`
}
