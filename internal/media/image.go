// Package media re-encodes user uploads so no EXIF/GPS or container metadata survives.
package media

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png" // register PNG decoder
	"io"

	"github.com/disintegration/imaging"
	"golang.org/x/image/riff"
	"golang.org/x/image/vp8"
	"golang.org/x/image/vp8l"
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
	if err := checkDims(cfg.Width, cfg.Height); err != nil {
		return nil, err
	}
	if isWebP(data) {
		// For VP8X files DecodeConfig reports the canvas size, but decoding allocates by
		// the inner frame size, so check the real frame header too.
		if err := checkWebPFrame(data, cfg.Width, cfg.Height); err != nil {
			return nil, err
		}
	}
	img, err := imaging.Decode(bytes.NewReader(data), imaging.AutoOrientation(true))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnsupportedFormat, err)
	}

	full := fitWithin(img, FullMaxEdge)
	thumb := fitWithin(full, ThumbMaxEdge)
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
	case isWebP(b):
		return true
	}
	return false
}

func isWebP(b []byte) bool {
	return len(b) >= 12 && bytes.Equal(b[:4], []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WEBP"))
}

func checkDims(w, h int) error {
	if w <= 0 || h <= 0 || int64(w)*int64(h) > MaxImagePixels {
		return ErrImageTooLarge
	}
	return nil
}

// checkWebPFrame walks the RIFF chunks and validates the real VP8/VP8L frame size against
// the pixel limit and the canvas size reported by DecodeConfig.
func checkWebPFrame(data []byte, canvasW, canvasH int) error {
	_, chunks, err := riff.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnsupportedFormat, err)
	}
	for {
		id, length, body, err := chunks.Next()
		if err == io.EOF {
			// no still-image frame (e.g. animated WebP)
			return fmt.Errorf("%w: no WebP frame", ErrUnsupportedFormat)
		}
		if err != nil {
			return fmt.Errorf("%w: %v", ErrUnsupportedFormat, err)
		}
		var w, h int
		switch id {
		case riff.FourCC{'V', 'P', '8', ' '}:
			d := vp8.NewDecoder()
			d.Init(body, int(length))
			fh, err := d.DecodeFrameHeader()
			if err != nil {
				return fmt.Errorf("%w: %v", ErrUnsupportedFormat, err)
			}
			w, h = fh.Width, fh.Height
		case riff.FourCC{'V', 'P', '8', 'L'}:
			c, err := vp8l.DecodeConfig(body)
			if err != nil {
				return fmt.Errorf("%w: %v", ErrUnsupportedFormat, err)
			}
			w, h = c.Width, c.Height
		default:
			continue
		}
		if err := checkDims(w, h); err != nil {
			return err
		}
		if w != canvasW || h != canvasH {
			return fmt.Errorf("%w: WebP frame size differs from canvas", ErrUnsupportedFormat)
		}
		return nil
	}
}

func fitWithin(img image.Image, maxEdge int) image.Image {
	b := img.Bounds()
	if b.Dx() <= maxEdge && b.Dy() <= maxEdge {
		return img
	}
	return imaging.Fit(img, maxEdge, maxEdge, imaging.Lanczos)
}

func encodeJPEG(img image.Image, quality int) ([]byte, error) {
	// JPEG has no alpha: flatten onto white so transparent pixels do not turn black.
	flat := image.NewRGBA(img.Bounds())
	draw.Draw(flat, flat.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(flat, flat.Bounds(), img, img.Bounds().Min, draw.Over)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, flat, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
