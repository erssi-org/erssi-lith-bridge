package weechatproto

import (
	"encoding/binary"
	"fmt"
	"io"
)

// ObjectType represents WeeChat protocol object types
type ObjectType string

const (
	TypeChar      ObjectType = "chr"
	TypeInteger   ObjectType = "int"
	TypeLong      ObjectType = "lon"
	TypeString    ObjectType = "str"
	TypeBuffer    ObjectType = "buf"
	TypePointer   ObjectType = "ptr"
	TypeTime      ObjectType = "tim"
	TypeHashTable ObjectType = "htb"
	TypeHData     ObjectType = "hda"
	TypeInfo      ObjectType = "inf"
	TypeInfoList  ObjectType = "inl"
	TypeArray     ObjectType = "arr"
)

// Message represents a WeeChat protocol message
type Message struct {
	ID          string
	Compression byte
	Data        []Object
}

// Object represents any WeeChat protocol object
type Object interface {
	Type() ObjectType
	Encode(w io.Writer) error
}

// Char represents a character
type Char struct {
	Value byte
}

func (c Char) Type() ObjectType { return TypeChar }
func (c Char) Encode(w io.Writer) error {
	_, err := w.Write([]byte{c.Value})
	return err
}

// Integer represents a 32-bit integer
type Integer struct {
	Value int32
}

func (i Integer) Type() ObjectType { return TypeInteger }
func (i Integer) Encode(w io.Writer) error {
	return binary.Write(w, binary.BigEndian, i.Value)
}

// Long represents a variable-length integer
type Long struct {
	Value int64
}

func (l Long) Type() ObjectType { return TypeLong }
func (l Long) Encode(w io.Writer) error {
	s := fmt.Sprintf("%d", l.Value)
	if err := binary.Write(w, binary.BigEndian, byte(len(s))); err != nil {
		return err
	}
	_, err := w.Write([]byte(s))
	return err
}

// String represents a string
type String struct {
	Value *string // nil for NULL strings
}

func (s String) Type() ObjectType { return TypeString }
func (s String) Encode(w io.Writer) error {
	if s.Value == nil {
		return binary.Write(w, binary.BigEndian, int32(-1))
	}
	if err := binary.Write(w, binary.BigEndian, int32(len(*s.Value))); err != nil {
		return err
	}
	if len(*s.Value) > 0 {
		_, err := w.Write([]byte(*s.Value))
		return err
	}
	return nil
}

// NewString creates a String with a value
func NewString(s string) String {
	return String{Value: &s}
}

// NullString creates a NULL string
func NullString() String {
	return String{Value: nil}
}

// Buffer represents binary data
type Buffer struct {
	Value []byte
}

func (b Buffer) Type() ObjectType { return TypeBuffer }
func (b Buffer) Encode(w io.Writer) error {
	if err := binary.Write(w, binary.BigEndian, int32(len(b.Value))); err != nil {
		return err
	}
	if len(b.Value) > 0 {
		_, err := w.Write(b.Value)
		return err
	}
	return nil
}

// Pointer represents a pointer (hex string)
type Pointer struct {
	Value string
}

func (p Pointer) Type() ObjectType { return TypePointer }
func (p Pointer) Encode(w io.Writer) error {
	if err := binary.Write(w, binary.BigEndian, byte(len(p.Value))); err != nil {
		return err
	}
	_, err := w.Write([]byte(p.Value))
	return err
}

// Time represents a timestamp
type Time struct {
	Value int64
}

func (t Time) Type() ObjectType { return TypeTime }
func (t Time) Encode(w io.Writer) error {
	s := fmt.Sprintf("%d", t.Value)
	if err := binary.Write(w, binary.BigEndian, byte(len(s))); err != nil {
		return err
	}
	_, err := w.Write([]byte(s))
	return err
}

// HashTable represents a hash table
type HashTable struct {
	KeyType   ObjectType
	ValueType ObjectType
	Count     int32
	Keys      []string
	Values    []string
}

func (h HashTable) Type() ObjectType { return TypeHashTable }
func (h HashTable) Encode(w io.Writer) error {
	// Write key type (3 bytes)
	if _, err := w.Write([]byte(h.KeyType)); err != nil {
		return err
	}
	// Write value type (3 bytes)
	if _, err := w.Write([]byte(h.ValueType)); err != nil {
		return err
	}
	// Write count
	if err := binary.Write(w, binary.BigEndian, h.Count); err != nil {
		return err
	}
	// Write key-value pairs
	for i := 0; i < int(h.Count); i++ {
		if err := NewString(h.Keys[i]).Encode(w); err != nil {
			return err
		}
		if err := NewString(h.Values[i]).Encode(w); err != nil {
			return err
		}
	}
	return nil
}

// HDataItem represents one item in HData
type HDataItem struct {
	Pointers []string
	Objects  map[string]Object
}

// HData represents hierarchical data
type HData struct {
	Path  string
	Keys  string
	Count int32
	Items []HDataItem
}

func (h HData) Type() ObjectType { return TypeHData }
func (h HData) Encode(w io.Writer) error {
	// Write hpath (path)
	if err := NewString(h.Path).Encode(w); err != nil {
		return err
	}
	// Write keys
	if err := NewString(h.Keys).Encode(w); err != nil {
		return err
	}
	// Write count
	if err := binary.Write(w, binary.BigEndian, h.Count); err != nil {
		return err
	}
	// Write items
	for _, item := range h.Items {
		// Write pointers
		for _, ptr := range item.Pointers {
			if err := Pointer{Value: ptr}.Encode(w); err != nil {
				return err
			}
		}
		// Write objects in key order
		// TODO: Parse keys and write objects in correct order
		for _, obj := range item.Objects {
			if err := obj.Encode(w); err != nil {
				return err
			}
		}
	}
	return nil
}

// Info represents an info value
type Info struct {
	Name  string
	Value string
}

func (i Info) Type() ObjectType { return TypeInfo }
func (i Info) Encode(w io.Writer) error {
	if err := NewString(i.Name).Encode(w); err != nil {
		return err
	}
	return NewString(i.Value).Encode(w)
}
