package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"mime"
	"mime/quotedprintable"
	netmail "net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/mailrat/mailrat/internal/gmail"
)

// SendService sends and replies to emails.
type SendService struct {
	client *gmail.Client
}

// NewSendService creates a SendService.
func NewSendService(client *gmail.Client) *SendService {
	return &SendService{client: client}
}

// Send sends an outbound message. Returns the new message ID.
func (s *SendService) Send(ctx context.Context, msg OutboundMessage) (string, error) {
	raw := buildRaw(msg)
	id, err := s.client.SendMessage(ctx, gmail.EncodeRaw(raw), msg.ThreadID)
	if err != nil {
		return "", err
	}
	return id, nil
}

// stripCRLF removes characters that could terminate a header line.
func stripCRLF(s string) string {
	return strings.NewReplacer("\r", "", "\n", "", "\x00", "").Replace(s)
}

// addrHeader parses an address list and re-serialises it with RFC 2047 display
// names. Unparseable input is kept with CR/LF stripped.
func addrHeader(v string) string {
	v = stripCRLF(v)
	list, err := netmail.ParseAddressList(v)
	if err != nil {
		return v
	}
	parts := make([]string, len(list))
	for i, a := range list {
		parts[i] = a.String()
	}
	return strings.Join(parts, ", ")
}

func writeHeader(b *strings.Builder, name, value string) {
	b.WriteString(name + ": " + value + "\r\n")
}

func messageIDDomain(from string) string {
	if a, err := netmail.ParseAddress(stripCRLF(from)); err == nil {
		if _, d, ok := strings.Cut(a.Address, "@"); ok && d != "" {
			return d
		}
	}
	return "mailrat.local"
}

// encodeBody returns the transfer-encoding name and encoded body (CRLF lines).
func encodeBody(s string) (string, string) {
	nonASCII := 0
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			nonASCII++
		}
	}
	if len(s) > 0 && nonASCII*3 > len(s) {
		enc := base64.StdEncoding.EncodeToString([]byte(s))
		var b strings.Builder
		for len(enc) > 76 {
			b.WriteString(enc[:76] + "\r\n")
			enc = enc[76:]
		}
		b.WriteString(enc + "\r\n")
		return "base64", b.String()
	}
	var buf bytes.Buffer
	w := quotedprintable.NewWriter(&buf)
	_, _ = w.Write([]byte(strings.ToValidUTF8(s, "\uFFFD")))
	_ = w.Close()
	out := buf.String()
	if !strings.HasSuffix(out, "\r\n") {
		out += "\r\n"
	}
	return "quoted-printable", out
}

func writePart(b *strings.Builder, boundary, ctype, body string) {
	cte, enc := encodeBody(body)
	if boundary != "" {
		b.WriteString("--" + boundary + "\r\n")
	}
	b.WriteString("Content-Type: " + ctype + "; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: " + cte + "\r\n\r\n")
	b.WriteString(enc)
}

// buildRaw constructs an RFC 5322 message. HTML bodies are sanitised and sent
// as multipart/alternative (text/plain then text/html).
func buildRaw(msg OutboundMessage) string {
	var b strings.Builder
	writeHeader(&b, "From", addrHeader(msg.From))
	writeHeader(&b, "To", addrHeader(msg.To))
	if msg.Cc != "" {
		writeHeader(&b, "Cc", addrHeader(msg.Cc))
	}
	if msg.Bcc != "" {
		writeHeader(&b, "Bcc", addrHeader(msg.Bcc))
	}
	writeHeader(&b, "Subject", mime.QEncoding.Encode("UTF-8", stripCRLF(msg.Subject)))
	writeHeader(&b, "Date", time.Now().Format(time.RFC1123Z))
	writeHeader(&b, "Message-ID", fmt.Sprintf("<%x.%x@%s>", time.Now().UnixNano(), randSuffix(), messageIDDomain(msg.From)))
	if irt := strings.TrimSpace(stripCRLF(msg.InReplyTo)); irt != "" {
		writeHeader(&b, "In-Reply-To", irt)
		writeHeader(&b, "References", irt)
	}
	writeHeader(&b, "MIME-Version", "1.0")

	if strings.HasPrefix(strings.TrimSpace(msg.Body), "<") {
		clean := SanitizeHTML(msg.Body)
		boundary := fmt.Sprintf("mailrat_%x", time.Now().UnixNano())
		b.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=%q\r\n\r\n", boundary))
		writePart(&b, boundary, "text/plain", htmlToText(clean))
		writePart(&b, boundary, "text/html", clean)
		b.WriteString("--" + boundary + "--\r\n")
	} else {
		writePart(&b, "", "text/plain", msg.Body)
	}
	return b.String()
}

func randSuffix() uint64 {
	var v [8]byte
	_, _ = rand.Read(v[:])
	return binary.BigEndian.Uint64(v[:])
}

// htmlToText converts HTML to plain text for the text/plain MIME part.
// Links become "text (url)", list items "- ", br/p/div line breaks.
func htmlToText(s string) string {
	doc, err := html.Parse(strings.NewReader(s))
	if err != nil {
		return strings.TrimSpace(s)
	}
	var buf strings.Builder
	var walk func(n *html.Node)
	nl := func() {
		if buf.Len() > 0 && !strings.HasSuffix(buf.String(), "\n") {
			buf.WriteByte('\n')
		}
	}
	walk = func(n *html.Node) {
		switch n.Type {
		case html.TextNode:
			buf.WriteString(n.Data)
			return
		case html.ElementNode:
			switch n.DataAtom {
			case atom.Script, atom.Style, atom.Head:
				return
			case atom.Br:
				buf.WriteByte('\n')
				return
			case atom.Hr:
				nl()
				buf.WriteString("----\n")
				return
			case atom.Li:
				nl()
				buf.WriteString("- ")
			case atom.A:
				start := buf.Len()
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c)
				}
				href := ""
				for _, a := range n.Attr {
					if a.Key == "href" {
						href = a.Val
					}
				}
				text := strings.TrimSpace(buf.String()[start:])
				if href != "" && text != href && !strings.HasPrefix(href, "mailto:"+text) {
					buf.WriteString(" (" + href + ")")
				}
				return
			case atom.P, atom.Div, atom.Blockquote, atom.Ul, atom.Ol, atom.Tr,
				atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6, atom.Pre, atom.Table:
				nl()
			case atom.Td, atom.Th:
				if buf.Len() > 0 && !strings.HasSuffix(buf.String(), "\n") {
					buf.WriteByte(' ')
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.Type == html.ElementNode {
			switch n.DataAtom {
			case atom.P, atom.Div, atom.Li, atom.Blockquote, atom.Tr,
				atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6, atom.Pre:
				nl()
			}
		}
	}
	walk(doc)
	out := strings.ReplaceAll(buf.String(), "\u00a0", " ")
	for strings.Contains(out, "\n\n\n") {
		out = strings.ReplaceAll(out, "\n\n\n", "\n\n")
	}
	if !utf8.ValidString(out) {
		out = strings.ToValidUTF8(out, "\uFFFD")
	}
	return strings.TrimSpace(out)
}
