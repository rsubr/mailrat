//go:build linux

package richclip

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/jezek/xgb/xproto"
)

// testXvfb starts a private X server so the tests never touch the real display.
func testXvfb(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("-short")
	}
	path, err := exec.LookPath("Xvfb")
	if err != nil {
		t.Skip("Xvfb not installed")
	}
	cmd := exec.Command(path, ":97", "-screen", "0", "320x200x24", "-nolisten", "tcp")
	if err := cmd.Start(); err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	for i := 0; i < 100; i++ {
		if _, err := os.Stat("/tmp/.X11-unix/X97"); err == nil {
			t.Setenv("DISPLAY", ":97")
			time.Sleep(100 * time.Millisecond)
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Skip("Xvfb :97 did not start (already in use?)")
}

func TestX11HTMLAndPlainTogether(t *testing.T) {
	testXvfb(t)
	html, text := "<b>héllo</b> <a href=\"https://e.com\">世界</a>", "héllo 世界"
	if err := (system{}).Set(html, text); err != nil {
		t.Fatal(err)
	}
	h, p, err := (system{}).ReadHTML()
	if err != nil || !strings.HasSuffix(h, html) || p != text {
		t.Fatalf("html %q plain %q err %v", h, p, err)
	}
	// Every advertised plain target is served from the same copy.
	x, err := dial()
	if err != nil {
		t.Fatal(err)
	}
	defer x.c.Close()
	clip := x.intern("CLIPBOARD")
	for _, tgt := range []string{"UTF8_STRING", "text/plain;charset=utf-8", "text/plain"} {
		if got := string(x.fetch(clip, tgt)); got != text {
			t.Errorf("%s: %q", tgt, got)
		}
	}
	if got := x.fetch(clip, "STRING"); string(got) != "h\xe9llo ??" {
		t.Errorf("STRING: %q", got)
	}
	if got := x.fetch(clip, "TARGETS"); len(got) < 8*4 {
		t.Errorf("TARGETS: % x", got)
	}
	if got := x.fetch(clip, "image/png"); got != nil {
		t.Errorf("unsupported target answered: %q", got)
	}
}

func TestX11OwnershipHandover(t *testing.T) {
	testXvfb(t)
	s := system{}
	if err := s.Set("<i>one</i>", "one"); err != nil {
		t.Fatal(err)
	}
	first := ownWin.Load()
	if err := s.Set("<i>two</i>", "two"); err != nil {
		t.Fatal(err)
	}
	if h, p, _ := s.ReadHTML(); !strings.HasSuffix(h, "<i>two</i>") || p != "two" {
		t.Fatalf("%q %q", h, p)
	}
	if ownWin.Load() == first {
		t.Fatal("owner window did not change")
	}
	// Another client takes the clipboard: our owner stops and ownWin clears.
	x, err := dial()
	if err != nil {
		t.Fatal(err)
	}
	defer x.c.Close()
	xproto.SetSelectionOwner(x.c, x.win, x.intern("CLIPBOARD"), xproto.TimeCurrentTime)
	for i := 0; i < 100 && ownWin.Load() != 0; i++ {
		time.Sleep(20 * time.Millisecond)
	}
	if ownWin.Load() != 0 {
		t.Fatal("old owner still serving after losing the selection")
	}
}

// Too large for one X request: Set degrades (callers copy plain text) and leaves the clipboard alone.
func TestX11LargePayloadDegrades(t *testing.T) {
	testXvfb(t)
	s := system{}
	if err := s.Set("<p>keep</p>", "keep"); err != nil {
		t.Fatal(err)
	}
	if err := s.Set("<p>"+strings.Repeat("x", 1<<20)+"</p>", "big"); err != ErrUnsupported {
		t.Fatalf("got %v", err)
	}
	if h, _, _ := s.ReadHTML(); !strings.HasSuffix(h, "<p>keep</p>") {
		t.Fatalf("clipboard changed: %.40q", h)
	}
	big := strings.Repeat("y", 150<<10)
	if err := s.Set("<p>"+big+"</p>", big); err != nil {
		t.Fatal(err)
	}
	if h, p, _ := s.ReadHTML(); !strings.HasSuffix(h, big+"</p>") || p != big {
		t.Fatalf("large copy lost: %d %d", len(h), len(p))
	}
}

func TestReadWithoutDisplay(t *testing.T) {
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	if _, _, err := (system{}).ReadHTML(); err != ErrUnsupported {
		t.Fatal(err)
	}
	if err := (system{}).Set("<b>x</b>", "x"); err != ErrUnsupported {
		t.Fatal(err)
	}
}
