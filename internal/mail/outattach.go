package mail

import (
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// MaxMessageBytes is Gmail's limit for a whole message, attachments included
// and counted after base64 encoding.
const MaxMessageBytes = 25 << 20

// Attachment is a file to attach to an outbound message. With Path set the
// bytes are read from disk only when the message is built (nothing is held in
// memory while composing); otherwise Data is the content (nil = empty file).
type Attachment struct {
	Filename string // display name; defaults to the base name of Path
	MIMEType string // optional; guessed from the extension or content when empty or invalid
	Path     string
	Data     []byte
}

// AttachmentSize returns the size in bytes of the attachment content.
func AttachmentSize(a Attachment) (int64, error) {
	if a.Path == "" {
		return int64(len(a.Data)), nil
	}
	fi, err := os.Stat(a.Path)
	if err != nil {
		return 0, fmt.Errorf("attachment %q: %w", a.Path, err)
	}
	if !fi.Mode().IsRegular() {
		return 0, fmt.Errorf("attachment %q is not a regular file", a.Path)
	}
	return fi.Size(), nil
}

// EncodedSize estimates the size of n bytes after base64 encoding with CRLF
// line breaks every 76 columns.
func EncodedSize(n int64) int64 {
	enc := (n + 2) / 3 * 4
	return enc + (enc+75)/76*2
}

// CheckAttachmentsSize returns an error when the attachments, once encoded,
// cannot fit in Gmail's message size limit together with bodyLen bytes of body.
func CheckAttachmentsSize(atts []Attachment, bodyLen int) error {
	total := int64(bodyLen)*2 + 4096 // plain + html copies and headers
	for _, a := range atts {
		n, err := AttachmentSize(a)
		if err != nil {
			return err
		}
		total += EncodedSize(n) + 512
	}
	if total > MaxMessageBytes {
		return fmt.Errorf("attachments are too large: the message would be about %.1f MB, Gmail's limit is %d MB",
			float64(total)/(1<<20), MaxMessageBytes>>20)
	}
	return nil
}

// attachmentName returns a safe display filename: no path, control characters
// or quotes. Empty results become "attachment".
func attachmentName(a Attachment) string {
	name := a.Filename
	if name == "" {
		name = a.Path
	}
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.Map(func(r rune) rune {
		switch {
		case r < 0x20 || r == 0x7f || r == 0x2028 || r == 0x2029:
			return -1
		case r == '"':
			return '\''
		}
		return r
	}, name)
	if name = strings.TrimSpace(name); name == "" || name == "." || name == ".." {
		return "attachment"
	}
	return name
}

// uniqueName makes name unique in used by adding " (2)", " (3)"... before the
// extension, and records it.
func uniqueName(used map[string]bool, name string) string {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for n := 2; used[strings.ToLower(name)]; n++ {
		name = fmt.Sprintf("%s (%d)%s", base, n, ext)
	}
	used[strings.ToLower(name)] = true
	return name
}

// attachmentType picks the Content-Type: the given one if valid, else by
// extension, else by sniffing the data, else application/octet-stream.
func attachmentType(a Attachment, name string, data []byte) string {
	if mt, _, err := mime.ParseMediaType(a.MIMEType); err == nil && strings.Contains(mt, "/") && !strings.HasPrefix(mt, "multipart/") {
		return mt
	}
	if mt, _, err := mime.ParseMediaType(mime.TypeByExtension(strings.ToLower(filepath.Ext(name)))); err == nil {
		return mt
	}
	if len(data) > 0 {
		if mt, _, err := mime.ParseMediaType(http.DetectContentType(data)); err == nil {
			return mt
		}
	}
	return "application/octet-stream"
}

// readAttachment returns the attachment bytes, reading Path from disk.
func readAttachment(a Attachment) ([]byte, error) {
	if a.Path == "" {
		return a.Data, nil
	}
	f, err := os.Open(a.Path)
	if err != nil {
		return nil, fmt.Errorf("attachment %q: %w", a.Path, err)
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("attachment %q: %w", a.Path, err)
	}
	return data, nil
}

// writeAttachment writes one attachment part (with its boundary line).
func writeAttachment(b *strings.Builder, boundary, name, ctype string, data []byte) {
	b.WriteString("--" + boundary + "\r\n")
	// FormatMediaType RFC 2231-encodes non-ASCII and special characters; the
	// name was already cleaned, so it cannot fail for a valid type.
	ct := mime.FormatMediaType(ctype, map[string]string{"name": name})
	cd := mime.FormatMediaType("attachment", map[string]string{"filename": name})
	if ct == "" {
		ct = "application/octet-stream"
	}
	if cd == "" {
		cd = "attachment"
	}
	writeHeader(b, "Content-Type", ct)
	writeHeader(b, "Content-Disposition", cd)
	writeHeader(b, "Content-Transfer-Encoding", "base64")
	b.WriteString("\r\n")
	writeBase64(b, data)
}

// writeBase64 writes data base64-encoded in 76-column CRLF lines.
func writeBase64(b *strings.Builder, data []byte) {
	const raw = 57 // 57 bytes -> 76 characters
	var line [76]byte
	for len(data) > 0 {
		n := min(raw, len(data))
		base64.StdEncoding.Encode(line[:], data[:n])
		b.Write(line[:(n+2)/3*4])
		b.WriteString("\r\n")
		data = data[n:]
	}
}
