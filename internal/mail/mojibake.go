package mail

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html/charset"
)

// mojibakeRun matches UTF-8 text that was read as Windows-1252 ("â€"" for an
// en dash): a lead character followed by the Windows-1252 renderings of
// continuation bytes 0x80-0xBF.
var mojibakeRun = regexp.MustCompile("[Â-ô][\u0080-¿ŒœŠšŸŽžƒˆ˜–—‘-‚“-„†-•…‰‹›€™]+")

// RepairMojibake undoes a UTF-8 -> Windows-1252 misdecode: each run that turns
// back into valid UTF-8 is replaced, anything else is left as it is.
func RepairMojibake(s string) string {
	if !strings.ContainsAny(s, "ÂÃÄÅÆÇÈÉÊËÌÍÎÏÐÑÒÓÔÕÖ×ØÙÚÛÜÝÞßàáâãäåæçèéêëìíîïðñòóô") {
		return s
	}
	enc, _ := charset.Lookup("windows-1252")
	if enc == nil {
		return s
	}
	return mojibakeRun.ReplaceAllStringFunc(s, func(run string) string {
		b, err := enc.NewEncoder().Bytes([]byte(run))
		if err != nil || !utf8.Valid(b) {
			return run
		}
		return string(b)
	})
}
