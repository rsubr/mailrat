package ui

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

func TestDownscaleImage(t *testing.T) {
	enc := func(w, h int) []byte {
		var b bytes.Buffer
		png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, w, h)))
		return b.Bytes()
	}
	small := enc(100, 50)
	if !bytes.Equal(downscaleImage(small, 1200), small) {
		t.Fatal("small image must pass through unchanged")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(downscaleImage(enc(2400, 1000), 1200)))
	if err != nil || cfg.Width != 1200 || cfg.Height != 500 {
		t.Fatalf("got %+v err %v, want 1200x500", cfg, err)
	}
	junk := []byte("not an image")
	if !bytes.Equal(downscaleImage(junk, 1200), junk) {
		t.Fatal("undecodable data must pass through")
	}
}
