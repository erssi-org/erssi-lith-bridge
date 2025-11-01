package erssiproto

import "encoding/json"

// MessageType represents erssi WebSocket message types
// erssi sends these as strings, not integers
type MessageType string

const (
	AuthOK              MessageType = "auth_ok"
	Message             MessageType = "message"
	ServerStatus        MessageType = "server_status"
	ChannelJoin         MessageType = "channel_join"
	ChannelPart         MessageType = "channel_part"
	ChannelKick         MessageType = "channel_kick"
	UserQuit            MessageType = "user_quit"
	Topic               MessageType = "topic"
	ChannelMode         MessageType = "channel_mode"
	Nicklist            MessageType = "nicklist"
	NicklistUpdate      MessageType = "nicklist_update"
	NickChange          MessageType = "nick_change"
	UserMode            MessageType = "user_mode"
	Away                MessageType = "away"
	Whois               MessageType = "whois"
	ChannelList         MessageType = "channel_list"
	StateDump           MessageType = "state_dump"
	SyncServer          MessageType = "sync_server"
	Error               MessageType = "error"
	Pong                MessageType = "pong"
	QueryOpened         MessageType = "query_opened"
	QueryClosed         MessageType = "query_closed"
	ActivityUpdate      MessageType = "activity_update"
	MarkRead            MessageType = "mark_read"
	NetworkList         MessageType = "network_list"
	NetworkListResponse MessageType = "network_list_response"
	ServerList          MessageType = "server_list"
	ServerListResponse  MessageType = "server_list_response"
	NetworkAdd          MessageType = "network_add"
	NetworkRemove       MessageType = "network_remove"
	ServerAdd           MessageType = "server_add"
	ServerRemove        MessageType = "server_remove"
	CommandResult       MessageType = "command_result"
)

// WebMessage represents a message from/to erssi fe-web
type WebMessage struct {
	ID             string                 `json:"id,omitempty"`
	Type           MessageType            `json:"type"`
	Server         string                 `json:"server,omitempty"`      // For sync_server requests
	ServerTag      string                 `json:"server_tag,omitempty"`  // For responses
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

// UnmarshalJSON implements custom JSON unmarshaling for WebMessage
// to handle erssi's inconsistent field naming (channel vs target, server vs server_tag)
func (m *WebMessage) UnmarshalJSON(data []byte) error {
	// Create a temporary struct with all possible field names
	type Alias WebMessage
	aux := &struct {
		Channel string `json:"channel,omitempty"` // Alternative to Target
		*Alias
	}{
		Alias: (*Alias)(m),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// If 'channel' field was present but 'target' wasn't, copy it over
	if aux.Channel != "" && m.Target == "" {
		m.Target = aux.Channel
	}

	// If 'server' field was present but 'server_tag' wasn't, copy it over
	if m.Server != "" && m.ServerTag == "" {
		m.ServerTag = m.Server
	}

	return nil
}
