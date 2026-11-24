package richclip

import (
	"bytes"
	"fmt"
	"strconv"
)

// CF_HTML ("HTML Format") is the Windows clipboard HTML flavour: a text header
// of byte offsets into a UTF-8 payload. The offsets count bytes, not runes.

const (
	cfStartMark = "<!--StartFragment-->"
	cfEndMark   = "<!--EndFragment-->"
	cfPrefix    = "<html><body>\r\n" + cfStartMark
	cfSuffix    = cfEndMark + "\r\n</body></html>"
)

// buildCFHTML wraps an HTML fragment in a CF_HTML header (without the trailing NUL
// that Windows wants after the data).
func buildCFHTML(fragment string) []byte {
	const hdr = "Version:0.9\r\nStartHTML:%010d\r\nEndHTML:%010d\r\nStartFragment:%010d\r\nEndFragment:%010d\r\n"
	n := len(fmt.Sprintf(hdr, 0, 0, 0, 0)) // fixed width: offsets are zero-padded
	startFrag := n + len(cfPrefix)
	endFrag := startFrag + len(fragment)
	endHTML := endFrag + len(cfSuffix)
	return []byte(fmt.Sprintf(hdr, n, endHTML, startFrag, endFrag) + cfPrefix + fragment + cfSuffix)
}

// parseCFHTML returns the HTML fragment of CF_HTML data. It trusts the
// StartFragment/EndFragment offsets when they are sane, else looks for the
// fragment markers, else returns whatever follows the header.
func parseCFHTML(b []byte) string {
	b = bytes.TrimRight(b, "\x00")
	field := func(name string) int {
		i := bytes.Index(b, []byte(name+":"))
		if i < 0 {
			return -1
		}
		rest := b[i+len(name)+1:]
		end := bytes.IndexAny(rest, "\r\n")
		if end < 0 {
			return -1
		}
		v, err := strconv.Atoi(string(bytes.TrimSpace(rest[:end])))
		if err != nil || v < 0 {
			return -1
		}
		return v
	}
	if s, e := field("StartFragment"), field("EndFragment"); s >= 0 && e >= s && e <= len(b) {
		return string(b[s:e])
	}
	if i := bytes.Index(b, []byte(cfStartMark)); i >= 0 {
		rest := b[i+len(cfStartMark):]
		if j := bytes.Index(rest, []byte(cfEndMark)); j >= 0 {
			return string(rest[:j])
		}
		return string(rest)
	}
	if s, e := field("StartHTML"), field("EndHTML"); s >= 0 && e >= s && e <= len(b) {
		return string(b[s:e])
	}
	if i := bytes.Index(b, []byte("\r\n\r\n")); i >= 0 {
		return string(b[i+4:])
	}
	return string(b)
}
