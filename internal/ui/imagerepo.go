package ui

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage/repository"
	xdraw "golang.org/x/image/draw"
)

const (
	maxRemoteImageWidth = 1200
	maxRemoteImageBytes = 10 << 20
)

type remoteImageRepo struct{}

// RegisterRemoteImages replaces Fyne's http(s) handler so remote mail images
// are downscaled before Fyne decodes them at full size.
func RegisterRemoteImages() {
	r := remoteImageRepo{}
	repository.Register("http", r)
	repository.Register("https", r)
}

type remoteImage struct {
	io.Reader
	uri fyne.URI
}

func (remoteImage) Close() error    { return nil }
func (i remoteImage) URI() fyne.URI { return i.uri }

func (remoteImageRepo) Reader(u fyne.URI) (fyne.URIReadCloser, error) {
	resp, err := http.Get(u.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, errors.New("remote server did not return a successful response")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxRemoteImageBytes))
	if err != nil {
		return nil, err
	}
	return remoteImage{bytes.NewReader(downscaleImage(data, maxRemoteImageWidth)), u}, nil
}

func (r remoteImageRepo) Exists(u fyne.URI) (bool, error) { return r.CanRead(u) }

func (remoteImageRepo) CanRead(u fyne.URI) (bool, error) {
	resp, err := http.Head(u.String())
	if err != nil {
		return false, err
	}
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return false, errors.New("remote server did not return a successful response")
	}
	return true, nil
}

func (remoteImageRepo) Destroy(string) {}

// downscaleImage shrinks a PNG or JPEG wider than maxW to maxW, keeping the
// aspect ratio. Anything else (GIF, SVG, undecodable, small) is returned as is.
func downscaleImage(data []byte, maxW int) []byte {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= maxW || (format != "png" && format != "jpeg") {
		return data
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return data
	}
	h := cfg.Height * maxW / cfg.Width
	if h < 1 {
		h = 1
	}
	dst := image.NewNRGBA(image.Rect(0, 0, maxW, h))
	xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Src, nil)
	var out bytes.Buffer
	if format == "jpeg" {
		err = jpeg.Encode(&out, dst, &jpeg.Options{Quality: 85})
	} else {
		err = png.Encode(&out, dst)
	}
	if err != nil {
		return data
	}
	return out.Bytes()
}
