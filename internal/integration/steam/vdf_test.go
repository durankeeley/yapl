package steam

import (
	"bytes"
	"testing"
)

// fixture is a hand-written binary VDF with every value type Steam uses.
func fixture() []byte {
	var b bytes.Buffer
	str := func(s string) { b.WriteString(s); b.WriteByte(0) }
	b.WriteByte(TypeMap)
	str("shortcuts")
	b.WriteByte(TypeMap)
	str("0")
	b.WriteByte(TypeInt32)
	str("appid")
	b.Write([]byte{0x01, 0x02, 0x03, 0x84})
	b.WriteByte(TypeString)
	str("AppName")
	str("My Emulator")
	b.WriteByte(TypeFloat32)
	str("volume")
	b.Write([]byte{0x00, 0x00, 0x80, 0x3f})
	b.WriteByte(TypeUint64)
	str("big")
	b.Write([]byte{1, 2, 3, 4, 5, 6, 7, 8})
	b.WriteByte(TypeMap)
	str("tags")
	b.WriteByte(TypeString)
	str("0")
	str("favorite")
	b.WriteByte(TypeEnd)
	b.WriteByte(TypeEnd)
	b.WriteByte(TypeEnd)
	b.WriteByte(TypeEnd)
	return b.Bytes()
}

func TestVDFRoundTrip(t *testing.T) {
	t.Run("given a file with every value type when parsed and encoded then the bytes are identical", func(t *testing.T) {
		// Given
		data := fixture()

		// When
		root, err := Parse(data)
		if err != nil {
			t.Fatal(err)
		}

		// Then
		if got := root.Encode(); !bytes.Equal(got, data) {
			t.Fatalf("round trip differs:\n got %x\nwant %x", got, data)
		}
	})

	t.Run("given a parsed file then values are reachable by key", func(t *testing.T) {
		root, _ := Parse(fixture())
		entry := root.Child("shortcuts").Child("0")
		if entry.String("AppName") != "My Emulator" || entry.Uint32("appid") != 0x84030201 || entry.Child("tags").String("0") != "favorite" {
			t.Fatalf("entry = %+v", entry)
		}
		if entry.Child("missing") != nil || entry.String("missing") != "" {
			t.Fatal("missing keys must read as empty")
		}
	})
}

func TestVDFErrors(t *testing.T) {
	data := fixture()
	tests := map[string][]byte{
		"truncated in the middle": data[:len(data)/2],
		"missing final end":       data[:len(data)-1],
		"unknown type":            {0x05, 'x', 0, TypeEnd},
		"unterminated key":        {TypeString, 'x'},
	}
	for name, input := range tests {
		t.Run("given a file "+name+" then parsing fails", func(t *testing.T) {
			if _, err := Parse(input); err == nil {
				t.Fatal("expected an error")
			}
		})
	}

	t.Run("given an empty file then it parses as an empty root", func(t *testing.T) {
		root, err := Parse([]byte{TypeEnd})
		if err != nil || len(root.Children) != 0 {
			t.Fatalf("root %+v err %v", root, err)
		}
	})
}

func TestVDFSetters(t *testing.T) {
	t.Run("given setters then existing keys are updated in place and new keys appended", func(t *testing.T) {
		n := &Node{Type: TypeMap}
		n.SetString("AppName", "a")
		n.SetUint32("appid", 7)
		n.SetString("AppName", "b")
		if len(n.Children) != 2 || n.String("AppName") != "b" || n.Children[0].Key != "AppName" || n.Uint32("appid") != 7 {
			t.Fatalf("node = %+v", n)
		}
	})
}
