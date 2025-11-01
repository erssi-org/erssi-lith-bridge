package weechatproto

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// Encoder encodes WeeChat protocol messages
type Encoder struct {
	writer io.Writer
}

// NewEncoder creates a new encoder
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{writer: w}
}

// EncodeMessage encodes a complete WeeChat message
func (e *Encoder) EncodeMessage(msg *Message) error {
	// Build message body first to calculate length
	bodyBuf := &bytes.Buffer{}

	// Write message ID (string)
	if err := NewString(msg.ID).Encode(bodyBuf); err != nil {
		return fmt.Errorf("failed to encode message ID: %w", err)
	}

	// Write objects
	for _, obj := range msg.Data {
		// Write type (3 bytes)
		typeStr := string(obj.Type())
		if len(typeStr) != 3 {
			return fmt.Errorf("invalid object type: %s (must be 3 chars)", typeStr)
		}
		if _, err := bodyBuf.Write([]byte(typeStr)); err != nil {
			return err
		}

		// Write object data
		if err := obj.Encode(bodyBuf); err != nil {
			return fmt.Errorf("failed to encode object type %s: %w", typeStr, err)
		}
	}

	body := bodyBuf.Bytes()

	// Calculate total length: 4 (length) + 1 (compression) + len(body)
	totalLen := uint32(4 + 1 + len(body))

	// Write length (4 bytes, big endian)
	if err := binary.Write(e.writer, binary.BigEndian, totalLen); err != nil {
		return fmt.Errorf("failed to write length: %w", err)
	}

	// Write compression (1 byte, 0 = none)
	if err := binary.Write(e.writer, binary.BigEndian, msg.Compression); err != nil {
		return fmt.Errorf("failed to write compression: %w", err)
	}

	// Write body
	if _, err := e.writer.Write(body); err != nil {
		return fmt.Errorf("failed to write body: %w", err)
	}

	return nil
}

// CreateHandshakeResponse creates a handshake response message
func CreateHandshakeResponse(id string, passwordHashAlgo string, nonce string) *Message {
	return &Message{
		ID:          id,
		Compression: 0,
		Data: []Object{
			HashTable{
				KeyType:   TypeString,
				ValueType: TypeString,
				Count:     6,
				Keys: []string{
					"password_hash_algo",
					"password_hash_iterations",
					"totp",
					"nonce",
					"compression",
					"escape_commands",
				},
				Values: []string{
					passwordHashAlgo,
					"100000",
					"off",
					nonce,
					"off",
					"off",
				},
			},
		},
	}
}

// CreateBuffersHData creates HData for buffer list
// id can be empty for responses to hdata requests, or "_buffer_opened" for broadcasts
func CreateBuffersHData(buffers []BufferData) *Message {
	return CreateBuffersHDataWithID(buffers, "")
}

// CreateBuffersHDataWithID creates HData for buffer list with custom message ID
func CreateBuffersHDataWithID(buffers []BufferData, id string) *Message {
	items := make([]HDataItem, len(buffers))

	for i, buf := range buffers {
		items[i] = HDataItem{
			Pointers: []string{buf.Pointer},
			Objects: map[string]Object{
				"number":           Integer{Value: buf.Number},
				"name":             NewString(buf.Name),
				"short_name":       NewString(buf.ShortName),
				"hidden":           Integer{Value: boolToInt(buf.Hidden)},
				"title":            NewString(buf.Title),
				"local_variables":  NewString(buf.LocalVariables),
			},
		}
	}

	return &Message{
		ID:          id,
		Compression: 0,
		Data: []Object{
			HData{
				Path:  "buffer",
				Keys:  "number:int,name:str,short_name:str,hidden:int,title:str,local_variables:str",
				Count: int32(len(items)),
				Items: items,
			},
		},
	}
}

// CreateEmptyHotlist creates an empty hotlist HData response
func CreateEmptyHotlist() *Message {
	return CreateEmptyHotlistWithID("")
}

// CreateEmptyHotlistWithID creates an empty hotlist HData response with custom message ID
func CreateEmptyHotlistWithID(id string) *Message {
	return &Message{
		ID:          id,
		Compression: 0,
		Data: []Object{
			HData{
				Path:  "hotlist",
				Keys:  "priority:int,date:tim,date_printed:tim,buffer:ptr,count:int",
				Count: 0,              // Empty hotlist
				Items: []HDataItem{}, // No items
			},
		},
	}
}

// BufferData represents buffer metadata
type BufferData struct {
	Pointer        string
	Number         int32
	Name           string
	ShortName      string
	Hidden         bool
	Title          string
	LocalVariables string
}

// CreateLinesHData creates HData for buffer lines
func CreateLinesHData(lines []LineData) *Message {
	return CreateLinesHDataWithID(lines, "")
}

// CreateLinesHDataWithID creates HData for buffer lines with custom message ID
func CreateLinesHDataWithID(lines []LineData, id string) *Message {
	items := make([]HDataItem, len(lines))

	for i, line := range lines {
		items[i] = HDataItem{
			Pointers: []string{line.Pointer},
			Objects: map[string]Object{
				"buffer":       Pointer{Value: line.BufferPtr},
				"date":         Time{Value: line.Date},
				"date_printed": Time{Value: line.DatePrinted},
				"displayed":    Integer{Value: boolToInt(line.Displayed)},
				"highlight":    Integer{Value: boolToInt(line.Highlight)},
				"tags_array":   NewString(line.Tags),
				"prefix":       NewString(line.Prefix),
				"message":      NewString(line.Message),
			},
		}
	}

	return &Message{
		ID:          id,
		Compression: 0,
		Data: []Object{
			HData{
				Path:  "line_data",
				Keys:  "buffer:ptr,date:tim,date_printed:tim,displayed:int,highlight:int,tags_array:str,prefix:str,message:str",
				Count: int32(len(items)),
				Items: items,
			},
		},
	}
}

// LineData represents a buffer line
type LineData struct {
	Pointer      string
	BufferPtr    string
	Date         int64
	DatePrinted  int64
	Displayed    bool
	Highlight    bool
	Tags         string
	Prefix       string
	Message      string
}

// CreateNicklistHData creates HData for nicklist
func CreateNicklistHData(nicks []NickData) *Message {
	items := make([]HDataItem, len(nicks))

	for i, nick := range nicks {
		items[i] = HDataItem{
			Pointers: []string{nick.Pointer},
			Objects: map[string]Object{
				"group":  Integer{Value: boolToInt(nick.IsGroup)},
				"visible": Integer{Value: boolToInt(nick.Visible)},
				"name":   NewString(nick.Name),
				"color":  NewString(nick.Color),
				"prefix": NewString(nick.Prefix),
				"prefix_color": NewString(nick.PrefixColor),
			},
		}
	}

	return &Message{
		ID:          "",
		Compression: 0,
		Data: []Object{
			HData{
				Path:  "nicklist_item",
				Keys:  "group:int,visible:int,name:str,color:str,prefix:str,prefix_color:str",
				Count: int32(len(items)),
				Items: items,
			},
		},
	}
}

// NickData represents a nick in nicklist
type NickData struct {
	Pointer      string
	IsGroup      bool
	Visible      bool
	Name         string
	Color        string
	Prefix       string
	PrefixColor  string
}

// Helper function to convert bool to int
func boolToInt(b bool) int32 {
	if b {
		return 1
	}
	return 0
}
