// Package media re-encodes user uploads so no EXIF/GPS or container metadata survives.
package media

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png" // register PNG decoder
	"io"

	"github.com/disintegration/imaging"
	_ "golang.org/x/image/webp" // register WebP decoder
)

const (
	MaxImagePixels = 50_000_000
	FullMaxEdge    = 2048
	ThumbMaxEdge   = 480

	fullQuality  = 85
	thumbQuality = 80
)

var (
	ErrUnsupportedFormat = errors.New("unsupported file format")
	ErrImageTooLarge     = errors.New("image dimensions too large")
	ErrVideoTooLong      = errors.New("video is longer than 60 seconds")
)

// ImageResult holds the re-encoded JPEGs. Width/Height describe Full after orientation.
type ImageResult struct {
	Full, Thumb   []byte
	Width, Height int
}

// ProcessImage sniffs the real format (JPEG/PNG/WebP), applies EXIF orientation and
// re-encodes full and thumbnail JPEGs without any metadata. The caller limits the size of r.
func ProcessImage(r io.Reader) (*ImageResult, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if !sniffImage(data) {
		return nil, ErrUnsupportedFormat
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnsupportedFormat, err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > MaxImagePixels {
		return nil, ErrImageTooLarge
	}
	img, err := imaging.Decode(bytes.NewReader(data), imaging.AutoOrientation(true))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnsupportedFormat, err)
	}

	full := fitWithin(img, FullMaxEdge)
	thumb := fitWithin(img, ThumbMaxEdge)
	fullBytes, err := encodeJPEG(full, fullQuality)
	if err != nil {
		return nil, err
	}
	thumbBytes, err := encodeJPEG(thumb, thumbQuality)
	if err != nil {
		return nil, err
	}
	b := full.Bounds()
	return &ImageResult{Full: fullBytes, Thumb: thumbBytes, Width: b.Dx(), Height: b.Dy()}, nil
}

func sniffImage(b []byte) bool {
	switch {
	case bytes.HasPrefix(b, []byte{0xFF, 0xD8, 0xFF}):
		return true
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")):
		return true
	case len(b) >= 12 && bytes.Equal(b[:4], []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WEBP")):
		return true
	}
	return false
}

func fitWithin(img image.Image, maxEdge int) image.Image {
	b := img.Bounds()
	if b.Dx() <= maxEdge && b.Dy() <= maxEdge {
		return img
	}
	return imaging.Fit(img, maxEdge, maxEdge, imaging.Lanczos)
}

func encodeJPEG(img image.Image, quality int) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
