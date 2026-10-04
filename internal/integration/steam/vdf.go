// Package steam adds YAPL games to Steam as non-Steam shortcuts, so Steam
// Big Picture (and the Steam Deck) can be the couch interface.
package steam

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
)

// Binary VDF value types.
const (
	TypeMap     byte = 0x00
	TypeString  byte = 0x01
	TypeInt32   byte = 0x02
	TypeFloat32 byte = 0x03
	TypeUint64  byte = 0x07
	TypeEnd     byte = 0x08
)

var numericSizes = map[byte]int{TypeInt32: 4, TypeFloat32: 4, TypeUint64: 8}

var errTruncated = errors.New("vdf: unexpected end of data")

// Node is one binary VDF entry. Numeric values keep their raw bytes so a
// file round-trips byte for byte, even for types YAPL never reads.
type Node struct {
	Type     byte
	Key      string
	Str      string
	Raw      []byte
	Children []*Node
}

// Parse reads a binary VDF file: a sequence of entries ended by TypeEnd.
func Parse(data []byte) (*Node, error) {
	r := &reader{data: data}
	root := &Node{Type: TypeMap}
	if err := r.readChildren(root); err != nil {
		return nil, err
	}
	if r.pos != len(data) {
		return nil, fmt.Errorf("vdf: %d trailing bytes", len(data)-r.pos)
	}
	return root, nil
}

// Encode writes the node's children followed by TypeEnd; for the root this
// is the whole file.
func (n *Node) Encode() []byte {
	var b bytes.Buffer
	for _, c := range n.Children {
		c.encode(&b)
	}
	b.WriteByte(TypeEnd)
	return b.Bytes()
}

func (n *Node) encode(b *bytes.Buffer) {
	b.WriteByte(n.Type)
	b.WriteString(n.Key)
	b.WriteByte(0)
	switch n.Type {
	case TypeMap:
		b.Write(n.Encode())
	case TypeString:
		b.WriteString(n.Str)
		b.WriteByte(0)
	default:
		b.Write(n.Raw)
	}
}

// Child returns the direct child with key, or nil.
func (n *Node) Child(key string) *Node {
	if n == nil {
		return nil
	}
	for _, c := range n.Children {
		if c.Key == key {
			return c
		}
	}
	return nil
}

// String returns a string child's value, or "".
func (n *Node) String(key string) string {
	if c := n.Child(key); c != nil && c.Type == TypeString {
		return c.Str
	}
	return ""
}

// Uint32 returns an int32 child's value as unsigned, or 0.
func (n *Node) Uint32(key string) uint32 {
	if c := n.Child(key); c != nil && c.Type == TypeInt32 && len(c.Raw) == 4 {
		return binary.LittleEndian.Uint32(c.Raw)
	}
	return 0
}

// SetString updates key in place or appends it.
func (n *Node) SetString(key, value string) {
	n.set(&Node{Type: TypeString, Key: key, Str: value})
}

// SetUint32 updates key (an int32 in VDF) in place or appends it.
func (n *Node) SetUint32(key string, value uint32) {
	raw := make([]byte, 4)
	binary.LittleEndian.PutUint32(raw, value)
	n.set(&Node{Type: TypeInt32, Key: key, Raw: raw})
}

func (n *Node) set(v *Node) {
	for i, c := range n.Children {
		if c.Key == v.Key {
			n.Children[i] = v
			return
		}
	}
	n.Children = append(n.Children, v)
}

type reader struct {
	data []byte
	pos  int
}

func (r *reader) readChildren(parent *Node) error {
	for {
		if r.pos >= len(r.data) {
			return errTruncated
		}
		typ := r.data[r.pos]
		r.pos++
		if typ == TypeEnd {
			return nil
		}
		key, err := r.cstring()
		if err != nil {
			return err
		}
		node := &Node{Type: typ, Key: key}
		switch typ {
		case TypeMap:
			err = r.readChildren(node)
		case TypeString:
			node.Str, err = r.cstring()
		default:
			size, ok := numericSizes[typ]
			if !ok {
				return fmt.Errorf("vdf: unknown type 0x%02x for key %q", typ, key)
			}
			if r.pos+size > len(r.data) {
				return errTruncated
			}
			node.Raw = append([]byte(nil), r.data[r.pos:r.pos+size]...)
			r.pos += size
		}
		if err != nil {
			return err
		}
		parent.Children = append(parent.Children, node)
	}
}

func (r *reader) cstring() (string, error) {
	end := bytes.IndexByte(r.data[r.pos:], 0)
	if end < 0 {
		return "", errTruncated
	}
	s := string(r.data[r.pos : r.pos+end])
	r.pos += end + 1
	return s, nil
}
