package richclip

import (
	"bytes"
	"encoding/binary"
	"unicode/utf16"
)

// decodeHTML turns clipboard HTML bytes into a UTF-8 string. Browsers offer
// text/html as UTF-16 on some platforms, with or without a byte order mark.
func decodeHTML(b []byte) string {
	var order binary.ByteOrder
	switch {
	case bytes.HasPrefix(b, []byte{0xff, 0xfe}):
		order, b = binary.LittleEndian, b[2:]
	case bytes.HasPrefix(b, []byte{0xfe, 0xff}):
		order, b = binary.BigEndian, b[2:]
	case len(b) >= 4 && b[1] == 0 && b[3] == 0 && b[0] != 0 && b[2] != 0:
		order = binary.LittleEndian // ASCII text such as "<h" in UTF-16LE
	case len(b) >= 4 && b[0] == 0 && b[2] == 0 && b[1] != 0 && b[3] != 0:
		order = binary.BigEndian
	}
	if order == nil {
		return string(bytes.TrimPrefix(b, []byte{0xef, 0xbb, 0xbf}))
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = order.Uint16(b[2*i:])
	}
	return string(utf16.Decode(u))
}
