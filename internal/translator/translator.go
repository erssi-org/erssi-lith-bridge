package translator

import (
	"fmt"
	"strconv"
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

// BufferState tracks state for a buffer (channel/query)
type BufferState struct {
	Pointer        string
	Number         int32
	ServerTag      string
	Name           string
	ShortName      string
	Title          string
	Lines          []weechatproto.LineData
	Nicks          []weechatproto.NickData
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

	// For now, create a simple buffer list
	// In real implementation, we'd parse the state dump
	buffers := make([]weechatproto.BufferData, 0)

	// Add core buffer
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
	}

	return weechatproto.CreateBuffersHData(buffers)
}

// ErssiMessageToLine converts erssi message to WeeChat line
func (t *Translator) ErssiMessageToLine(msg *erssiproto.WebMessage) *weechatproto.Message {
	t.buffersMu.Lock()
	defer t.buffersMu.Unlock()

	// Find or create buffer
	bufferKey := fmt.Sprintf("%s.%s", msg.ServerTag, msg.Target)
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

	// Add to buffer lines
	buffer.Lines = append(buffer.Lines, line)

	// Create HData message
	return weechatproto.CreateLinesHData([]weechatproto.LineData{line})
}

// ErssiNicklistToWeeChat converts erssi nicklist to WeeChat format
func (t *Translator) ErssiNicklistToWeeChat(msg *erssiproto.WebMessage, nicks []erssiproto.NickInfo) *weechatproto.Message {
	t.buffersMu.Lock()
	defer t.buffersMu.Unlock()

	// Find buffer
	bufferKey := fmt.Sprintf("%s.%s", msg.ServerTag, msg.Target)
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
	num := t.nextBufferNum
	t.nextBufferNum++

	buffer := &BufferState{
		Pointer:   t.generatePointer(),
		Number:    num,
		ServerTag: serverTag,
		Name:      fmt.Sprintf("%s.%s", serverTag, target),
		ShortName: target,
		Lines:     make([]weechatproto.LineData, 0),
		Nicks:     make([]weechatproto.NickData, 0),
	}

	bufferKey := fmt.Sprintf("%s.%s", serverTag, target)
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

// GetAllBuffers returns all buffers as WeeChat HData
func (t *Translator) GetAllBuffers() *weechatproto.Message {
	t.buffersMu.RLock()
	defer t.buffersMu.RUnlock()

	buffers := make([]weechatproto.BufferData, 0, len(t.buffers))

	for _, buf := range t.buffers {
		buffers = append(buffers, weechatproto.BufferData{
			Pointer:        buf.Pointer,
			Number:         buf.Number,
			Name:           buf.Name,
			ShortName:      buf.ShortName,
			Hidden:         false,
			Title:          buf.Title,
			LocalVariables: "type=channel",
		})
	}

	return weechatproto.CreateBuffersHData(buffers)
}

// GetBufferLines returns lines for a buffer
func (t *Translator) GetBufferLines(bufferPtr string, count int) *weechatproto.Message {
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

			return weechatproto.CreateLinesHData(lines)
		}
	}

	// Return empty if buffer not found
	return weechatproto.CreateLinesHData([]weechatproto.LineData{})
}
