package translator

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"erssi-lith-bridge/pkg/erssiproto"
	"erssi-lith-bridge/pkg/weechatproto"

	"github.com/sirupsen/logrus"
)

// Translator converts between erssi and WeeChat protocols
type Translator struct {
	log *logrus.Entry

	// State management
	buffers   map[string]*BufferState
	buffersMu sync.RWMutex

	nextBufferNum int32
}

// BufferState tracks state for a buffer (channel/query/server)
type BufferState struct {
	Pointer   string
	Number    int32
	ServerTag string
	Name      string
	ShortName string
	Title     string
	Lines     []weechatproto.LineData
	Nicks     []weechatproto.NickData
	IsServer  bool // True if this is a server buffer (not a channel)
}

// NewTranslator creates a new protocol translator
func NewTranslator(logger *logrus.Logger) *Translator {
	if logger == nil {
		logger = logrus.New()
	}

	return &Translator{
		log:           logger.WithField("component", "translator"),
		buffers:       make(map[string]*BufferState),
		nextBufferNum: 1,
	}
}

// ErssiToBufferList converts erssi state dump to WeeChat buffer list
func (t *Translator) ErssiToBufferList(stateDump *erssiproto.WebMessage) *weechatproto.Message {
	t.buffersMu.Lock()
	defer t.buffersMu.Unlock()

	t.log.Debug("Parsing state dump...")

	// Parse state dump - try both ExtraData and Text fields
	var parsedData interface{}

	if stateDump.ExtraData != nil && len(stateDump.ExtraData) > 0 {
		parsedData = stateDump.ExtraData
		t.log.Debug("Using ExtraData for state dump")
	} else if stateDump.Text != "" {
		// Try to parse Text field as JSON
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(stateDump.Text), &data); err == nil {
			parsedData = data
			t.log.Debug("Using Text field for state dump")
		}
	}

	buffers := make([]weechatproto.BufferData, 0)

	// Add core buffer first
	corePtr := t.generatePointer()
	buffers = append(buffers, weechatproto.BufferData{
		Pointer:        corePtr,
		Number:         1,
		Name:           "core.weechat",
		ShortName:      "weechat",
		Hidden:         false,
		Title:          "WeeChat (via erssi bridge)",
		LocalVariables: "type=server",
	})

	t.buffers["core"] = &BufferState{
		Pointer:   corePtr,
		Number:    1,
		Name:      "core.weechat",
		ShortName: "weechat",
		Lines:     make([]weechatproto.LineData, 0),
		Nicks:     make([]weechatproto.NickData, 0),
	}

	// Parse servers structure
	if dataMap, ok := parsedData.(map[string]interface{}); ok {
		if serversData, ok := dataMap["servers"]; ok {
			if serversList, ok := serversData.([]interface{}); ok {
				for _, serverItem := range serversList {
					if server, ok := serverItem.(map[string]interface{}); ok {
						serverTag := getString(server, "tag")
						if serverTag == "" {
							continue
						}

						t.log.Debugf("Processing server: %s", serverTag)

						// Process channels
						if channelsData, ok := server["channels"].([]interface{}); ok {
							for _, channelItem := range channelsData {
								if channel, ok := channelItem.(map[string]interface{}); ok {
									channelName := getString(channel, "name")
									topic := getString(channel, "topic")

									if channelName != "" {
										buffer := t.createBufferWithTopic(serverTag, channelName, topic)
										buffers = append(buffers, weechatproto.BufferData{
											Pointer:        buffer.Pointer,
											Number:         buffer.Number,
											Name:           buffer.Name,
											ShortName:      buffer.ShortName,
											Hidden:         false,
											Title:          buffer.Title,
											LocalVariables: "type=channel",
										})
										t.log.Debugf("Created buffer for channel: %s.%s", serverTag, channelName)
									}
								}
							}
						}

						// Process queries
						if queriesData, ok := server["queries"].([]interface{}); ok {
							for _, queryItem := range queriesData {
								if query, ok := queryItem.(map[string]interface{}); ok {
									nick := getString(query, "nick")

									if nick != "" {
										buffer := t.createBufferWithTopic(serverTag, nick, "")
										buffers = append(buffers, weechatproto.BufferData{
											Pointer:        buffer.Pointer,
											Number:         buffer.Number,
											Name:           buffer.Name,
											ShortName:      buffer.ShortName,
											Hidden:         false,
											Title:          fmt.Sprintf("Private chat with %s", nick),
											LocalVariables: "type=private",
										})
										t.log.Debugf("Created buffer for query: %s.%s", serverTag, nick)
									}
								}
							}
						}
					}
				}
			}
		}
	}

	t.log.Infof("Created %d buffers from state dump", len(buffers))
	return weechatproto.CreateBuffersHData(buffers)
}

// ErssiMessageToLine converts erssi message to WeeChat line
func (t *Translator) ErssiMessageToLine(msg *erssiproto.WebMessage) *weechatproto.Message {
	t.buffersMu.Lock()
	defer t.buffersMu.Unlock()

	// Find or create buffer (normalize key)
	normalizedTarget := strings.ToLower(msg.Target)
	bufferKey := fmt.Sprintf("%s.%s", msg.ServerTag, normalizedTarget)
	buffer, ok := t.buffers[bufferKey]
	if !ok {
		// Create new buffer
		buffer = t.createBuffer(msg.ServerTag, msg.Target)
	}

	// Create line data
	line := weechatproto.LineData{
		Pointer:      t.generatePointer(),
		BufferPtr:    buffer.Pointer,
		Date:         msg.Timestamp,
		DatePrinted:  time.Now().Unix(),
		Displayed:    true,
		Highlight:    msg.IsHighlight,
		Tags:         t.generateTags(msg),
		Prefix:       msg.Nick,
		Message:      msg.Text,
	}

	// Add to buffer lines (keep last 500 lines for history)
	buffer.Lines = append(buffer.Lines, line)
	if len(buffer.Lines) > 500 {
		buffer.Lines = buffer.Lines[len(buffer.Lines)-500:]
	}

	// Create HData message
	return weechatproto.CreateLinesHData([]weechatproto.LineData{line})
}

// ErssiNicklistToWeeChat converts erssi nicklist to WeeChat format
func (t *Translator) ErssiNicklistToWeeChat(msg *erssiproto.WebMessage, nicks []erssiproto.NickInfo) *weechatproto.Message {
	t.buffersMu.Lock()
	defer t.buffersMu.Unlock()

	// Find buffer (normalize key)
	normalizedTarget := strings.ToLower(msg.Target)
	bufferKey := fmt.Sprintf("%s.%s", msg.ServerTag, normalizedTarget)
	buffer, ok := t.buffers[bufferKey]
	if !ok {
		buffer = t.createBuffer(msg.ServerTag, msg.Target)
	}

	// Convert nicks
	nickData := make([]weechatproto.NickData, len(nicks))
	for i, nick := range nicks {
		nickData[i] = weechatproto.NickData{
			Pointer:      t.generatePointer(),
			IsGroup:      false,
			Visible:      true,
			Name:         nick.Nick,
			Color:        "default",
			Prefix:       nick.Prefix,
			PrefixColor:  t.getPrefixColor(nick.Prefix),
		}
	}

	// Update buffer state
	buffer.Nicks = nickData

	return weechatproto.CreateNicklistHData(nickData)
}

// WeeChat command parsing

// ParseInputCommand parses WeeChat input command
// Format: input <buffer_pointer> <text>
func (t *Translator) ParseInputCommand(args []string) (bufferPtr, text string, err error) {
	if len(args) < 2 {
		return "", "", fmt.Errorf("invalid input command: need buffer and text")
	}

	bufferPtr = args[0]
	text = strings.Join(args[1:], " ")

	return bufferPtr, text, nil
}

// ParseHDataCommand parses WeeChat hdata command
// Format: hdata <path> [<arguments>]
func (t *Translator) ParseHDataCommand(args []string) (path string, params string, err error) {
	if len(args) < 1 {
		return "", "", fmt.Errorf("invalid hdata command: need path")
	}

	path = args[0]
	if len(args) > 1 {
		params = args[1]
	}

	return path, params, nil
}

// WeeChat to erssi conversion

// InputToErssiCommand converts WeeChat input to erssi command
func (t *Translator) InputToErssiCommand(bufferPtr, text string) (*erssiproto.WebMessage, error) {
	t.buffersMu.RLock()
	defer t.buffersMu.RUnlock()

	// Find buffer by pointer
	var serverTag, target string
	for key, buf := range t.buffers {
		if buf.Pointer == bufferPtr {
			parts := strings.SplitN(key, ".", 2)
			if len(parts) == 2 {
				serverTag = parts[0]
				target = parts[1]
			}
			break
		}
	}

	if serverTag == "" {
		return nil, fmt.Errorf("buffer not found: %s", bufferPtr)
	}

	return &erssiproto.WebMessage{
		Type:      erssiproto.Message,
		ServerTag: serverTag,
		Target:    target,
		Text:      text,
	}, nil
}

// Helper methods

func (t *Translator) createBuffer(serverTag, target string) *BufferState {
	return t.createBufferWithTopic(serverTag, target, "")
}

// EnsureServerBuffer creates a server buffer if it doesn't exist (thread-safe, public)
func (t *Translator) EnsureServerBuffer(serverTag string) *BufferState {
	t.buffersMu.Lock()
	defer t.buffersMu.Unlock()

	// Server buffer key is just the server tag
	bufferKey := serverTag

	// Check if server buffer already exists
	if existing, ok := t.buffers[bufferKey]; ok {
		return existing
	}

	num := t.nextBufferNum
	t.nextBufferNum++

	buffer := &BufferState{
		Pointer:   t.generatePointer(),
		Number:    num,
		ServerTag: serverTag,
		Name:      serverTag,
		ShortName: serverTag,
		Title:     fmt.Sprintf("Server %s", serverTag),
		Lines:     make([]weechatproto.LineData, 0),
		Nicks:     make([]weechatproto.NickData, 0),
		IsServer:  true, // Mark as server buffer
	}

	t.buffers[bufferKey] = buffer

	t.log.Debugf("Created server buffer: %s (ptr=%s, num=%d)", bufferKey, buffer.Pointer, buffer.Number)

	return buffer
}

// EnsureBuffer creates a buffer if it doesn't exist (thread-safe, public)
func (t *Translator) EnsureBuffer(serverTag, target string) *BufferState {
	t.buffersMu.Lock()
	defer t.buffersMu.Unlock()

	return t.createBufferWithTopic(serverTag, target, "")
}

func (t *Translator) createBufferWithTopic(serverTag, target, topic string) *BufferState {
	// Normalize channel name for key
	normalizedTarget := strings.ToLower(target)
	bufferKey := fmt.Sprintf("%s.%s", serverTag, normalizedTarget)

	// Check if buffer already exists
	if existing, ok := t.buffers[bufferKey]; ok {
		// Update topic if provided
		if topic != "" {
			existing.Title = topic
		}
		return existing
	}

	num := t.nextBufferNum
	t.nextBufferNum++

	buffer := &BufferState{
		Pointer:   t.generatePointer(),
		Number:    num,
		ServerTag: serverTag,
		Name:      fmt.Sprintf("%s.%s", serverTag, target),
		ShortName: target,
		Title:     topic,
		Lines:     make([]weechatproto.LineData, 0),
		Nicks:     make([]weechatproto.NickData, 0),
	}

	t.buffers[bufferKey] = buffer

	t.log.Debugf("Created buffer: %s (ptr=%s, num=%d)", bufferKey, buffer.Pointer, buffer.Number)

	return buffer
}

func (t *Translator) generatePointer() string {
	// Generate a fake pointer (hex string)
	return fmt.Sprintf("0x%x", time.Now().UnixNano())
}

func (t *Translator) generateTags(msg *erssiproto.WebMessage) string {
	tags := []string{}

	// Add standard tags
	tags = append(tags, "notify_message")

	if msg.IsHighlight {
		tags = append(tags, "notify_highlight")
	}

	if msg.Nick != "" {
		tags = append(tags, fmt.Sprintf("nick_%s", msg.Nick))
	}

	return strings.Join(tags, ",")
}

func (t *Translator) getPrefixColor(prefix string) string {
	switch prefix {
	case "@":
		return "lightgreen"
	case "+":
		return "yellow"
	case "%":
		return "lightmagenta"
	default:
		return "default"
	}
}

// GetAllBuffers returns all buffers as WeeChat HData (for responding to hdata requests)
func (t *Translator) GetAllBuffers(msgID string) *weechatproto.Message {
	t.buffersMu.RLock()
	defer t.buffersMu.RUnlock()

	// Collect all buffers and sort by number (server buffers first, then channels)
	bufferList := make([]*BufferState, 0, len(t.buffers))
	for _, buf := range t.buffers {
		bufferList = append(bufferList, buf)
	}

	// Sort by buffer number
	sort.Slice(bufferList, func(i, j int) bool {
		return bufferList[i].Number < bufferList[j].Number
	})

	buffers := make([]weechatproto.BufferData, 0, len(bufferList))

	for _, buf := range bufferList {
		// Set local_variables based on buffer type
		localVars := "type=channel,server=" + buf.ServerTag
		if buf.IsServer {
			localVars = "type=server"
		}

		buffers = append(buffers, weechatproto.BufferData{
			Pointer:        buf.Pointer,
			Number:         buf.Number,
			Name:           buf.Name,
			ShortName:      buf.ShortName,
			Hidden:         false,
			Title:          buf.Title,
			LocalVariables: localVars,
		})
	}

	return weechatproto.CreateBuffersHDataWithID(buffers, msgID)
}

// getBufferKey returns the buffer key for a server and target
func getBufferKey(serverTag, target string) string {
	normalizedTarget := strings.ToLower(target)
	return fmt.Sprintf("%s.%s", serverTag, normalizedTarget)
}

// GetBufferOpenedEvent returns _buffer_opened event for a single buffer
func (t *Translator) GetBufferOpenedEvent(serverTag, target string) *weechatproto.Message {
	t.buffersMu.RLock()
	defer t.buffersMu.RUnlock()

	bufferKey := getBufferKey(serverTag, target)

	if buf, exists := t.buffers[bufferKey]; exists {
		// Set local_variables based on buffer type
		localVars := "type=channel,server=" + buf.ServerTag
		if buf.IsServer {
			localVars = "type=server"
		}

		buffers := []weechatproto.BufferData{{
			Pointer:        buf.Pointer,
			Number:         buf.Number,
			Name:           buf.Name,
			ShortName:      buf.ShortName,
			Hidden:         false,
			Title:          buf.Title,
			LocalVariables: localVars,
		}}
		return weechatproto.CreateBuffersHDataWithID(buffers, "_buffer_opened")
	}

	// Return empty if buffer not found
	return weechatproto.CreateBuffersHDataWithID([]weechatproto.BufferData{}, "_buffer_opened")
}

// GetBufferList returns list of buffer pointers for counting
func (t *Translator) GetBufferList() []string {
	t.buffersMu.RLock()
	defer t.buffersMu.RUnlock()

	result := make([]string, 0, len(t.buffers))
	for ptr := range t.buffers {
		result = append(result, ptr)
	}
	return result
}

// GetEmptyHotlist returns an empty hotlist response
func (t *Translator) GetEmptyHotlist(msgID string) *weechatproto.Message {
	// Return empty hotlist HData
	return weechatproto.CreateEmptyHotlistWithID(msgID)
}

// GetBufferLines returns lines for a buffer
func (t *Translator) GetBufferLines(bufferPtr string, count int, msgID string) *weechatproto.Message {
	t.buffersMu.RLock()
	defer t.buffersMu.RUnlock()

	for _, buf := range t.buffers {
		if buf.Pointer == bufferPtr {
			// Return last N lines
			start := 0
			if len(buf.Lines) > count {
				start = len(buf.Lines) - count
			}
			lines := buf.Lines[start:]

			return weechatproto.CreateLinesHDataWithID(lines, msgID)
		}
	}

	// Return empty if buffer not found
	return weechatproto.CreateLinesHDataWithID([]weechatproto.LineData{}, msgID)
}

// GetBufferInfo returns server tag and target for a buffer pointer
func (t *Translator) GetBufferInfo(bufferPtr string) (serverTag, target string) {
	t.buffersMu.RLock()
	defer t.buffersMu.RUnlock()

	for key, buf := range t.buffers {
		if buf.Pointer == bufferPtr {
			parts := strings.SplitN(key, ".", 2)
			if len(parts) == 2 {
				return parts[0], parts[1]
			}
			return buf.ServerTag, buf.ShortName
		}
	}

	return "", ""
}

// getString safely extracts a string from a map
func getString(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}
