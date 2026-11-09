package media

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"
)

func testJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 4), uint8(y * 4), 128, 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewGray(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetGray(x, y, color.Gray{uint8(x + y)})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// withExif inserts an APP1 Exif segment (IFD0 orientation, optional GPS IFD pointer) after SOI.
func withExif(jpegData []byte, orientation int, gps bool) []byte {
	le := binary.LittleEndian
	var tiff bytes.Buffer
	tiff.WriteString("II")
	_ = binary.Write(&tiff, le, uint16(42))
	_ = binary.Write(&tiff, le, uint32(8))
	n := 1
	if gps {
		n = 2
	}
	_ = binary.Write(&tiff, le, uint16(n))
	// orientation entry
	_ = binary.Write(&tiff, le, uint16(0x0112))
	_ = binary.Write(&tiff, le, uint16(3))
	_ = binary.Write(&tiff, le, uint32(1))
	_ = binary.Write(&tiff, le, uint16(orientation))
	_ = binary.Write(&tiff, le, uint16(0))
	if gps {
		gpsOffset := uint32(8 + 2 + 12*2 + 4)
		_ = binary.Write(&tiff, le, uint16(0x8825))
		_ = binary.Write(&tiff, le, uint16(4))
		_ = binary.Write(&tiff, le, uint32(1))
		_ = binary.Write(&tiff, le, gpsOffset)
	}
	_ = binary.Write(&tiff, le, uint32(0)) // next IFD
	if gps {
		// GPS IFD: one entry, GPSLatitudeRef = "N"
		_ = binary.Write(&tiff, le, uint16(1))
		_ = binary.Write(&tiff, le, uint16(1))
		_ = binary.Write(&tiff, le, uint16(2))
		_ = binary.Write(&tiff, le, uint32(2))
		tiff.Write([]byte{'N', 0, 0, 0})
		_ = binary.Write(&tiff, le, uint32(0))
	}
	payload := append([]byte("Exif\x00\x00"), tiff.Bytes()...)
	seg := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(seg[2:], uint16(len(payload)+2))
	seg = append(seg, payload...)

	out := append([]byte{}, jpegData[:2]...)
	out = append(out, seg...)
	return append(out, jpegData[2:]...)
}

func TestProcessImage_StripsExifAndGPS(t *testing.T) {
	in := withExif(testJPEG(t, 64, 32), 1, true)
	if !bytes.Contains(in, []byte("Exif")) {
		t.Fatal("test input should contain Exif")
	}
	res, err := ProcessImage(bytes.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	for name, b := range map[string][]byte{"full": res.Full, "thumb": res.Thumb} {
		if bytes.Contains(b, []byte("Exif")) {
			t.Errorf("%s contains Exif", name)
		}
		if bytes.Contains(b, []byte{0xFF, 0xE1}) {
			t.Errorf("%s contains APP1 marker", name)
		}
		if _, err := jpeg.Decode(bytes.NewReader(b)); err != nil {
			t.Errorf("%s is not a valid JPEG: %v", name, err)
		}
	}
}

// cornerJPEG is 64x32 blue with a red 16x16 block in the top-left corner.
func cornerJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 64, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 64; x++ {
			if x < 16 && y < 16 {
				img.Set(x, y, color.RGBA{255, 0, 0, 255})
			} else {
				img.Set(x, y, color.RGBA{0, 0, 255, 255})
			}
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func isRed(c color.Color) bool {
	r, _, b, _ := c.RGBA()
	return r>>8 > 180 && b>>8 < 90
}

func TestProcessImage_AppliesOrientation(t *testing.T) {
	cases := []struct {
		orientation int
		redX, redY  int // a pixel inside where the red corner must land in the 32x64 result
	}{
		{6, 24, 8}, // rotate 90 CW: top-left -> top-right
		{8, 8, 56}, // rotate 90 CCW: top-left -> bottom-left
	}
	for _, c := range cases {
		res, err := ProcessImage(bytes.NewReader(withExif(cornerJPEG(t), c.orientation, false)))
		if err != nil {
			t.Fatal(err)
		}
		if res.Width != 32 || res.Height != 64 {
			t.Fatalf("orientation %d: got %dx%d, want 32x64", c.orientation, res.Width, res.Height)
		}
		img, err := jpeg.Decode(bytes.NewReader(res.Full))
		if err != nil {
			t.Fatal(err)
		}
		if !isRed(img.At(c.redX, c.redY)) {
			t.Errorf("orientation %d: expected red at (%d,%d), got %v", c.orientation, c.redX, c.redY, img.At(c.redX, c.redY))
		}
		if isRed(img.At(8, 8)) && c.orientation == 6 {
			t.Errorf("orientation 6: top-left should not be red")
		}
	}
}

func TestProcessImage_FlattensTransparencyOnWhite(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16)) // fully transparent
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	res, err := ProcessImage(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	out, err := jpeg.Decode(bytes.NewReader(res.Full))
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := out.At(8, 8).RGBA()
	if r>>8 < 240 || g>>8 < 240 || b>>8 < 240 {
		t.Fatalf("transparent pixel not white: %v", out.At(8, 8))
	}
}

// webpVP8X builds a RIFF/WEBP with a VP8X canvas chunk and a VP8L chunk whose header
// declares frameW x frameH. Only headers are present, so the file is tiny.
func webpVP8X(canvasW, canvasH, frameW, frameH int) []byte {
	vp8x := make([]byte, 10)
	put24 := func(b []byte, v int) { b[0], b[1], b[2] = byte(v), byte(v>>8), byte(v>>16) }
	put24(vp8x[4:], canvasW-1)
	put24(vp8x[7:], canvasH-1)
	vp8l := make([]byte, 5)
	vp8l[0] = 0x2f
	bits := uint32(frameW-1) | uint32(frameH-1)<<14
	binary.LittleEndian.PutUint32(vp8l[1:], bits)

	var body bytes.Buffer
	body.WriteString("WEBP")
	chunk := func(id string, data []byte) {
		body.WriteString(id)
		_ = binary.Write(&body, binary.LittleEndian, uint32(len(data)))
		body.Write(data)
		if len(data)%2 == 1 {
			body.WriteByte(0)
		}
	}
	chunk("VP8X", vp8x)
	chunk("VP8L", vp8l)
	var out bytes.Buffer
	out.WriteString("RIFF")
	_ = binary.Write(&out, binary.LittleEndian, uint32(body.Len()))
	out.Write(body.Bytes())
	return out.Bytes()
}

func TestProcessImage_RejectsWebPCanvasFrameMismatch(t *testing.T) {
	// canvas says 1x1 (passes DecodeConfig) but the VP8L frame says 16384x16384.
	// Must be rejected from headers alone, before any pixel allocation.
	data := webpVP8X(1, 1, 16384, 16384)
	if len(data) > 100 {
		t.Fatalf("crafted file unexpectedly large: %d", len(data))
	}
	if _, err := ProcessImage(bytes.NewReader(data)); !errors.Is(err, ErrImageTooLarge) {
		t.Fatalf("got %v, want ErrImageTooLarge", err)
	}
	// small frame that disagrees with canvas is unsupported
	if _, err := ProcessImage(bytes.NewReader(webpVP8X(4, 4, 2, 2))); !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("got %v, want ErrUnsupportedFormat", err)
	}
}

func TestProcessImage_ResizesLargeAndKeepsSmall(t *testing.T) {
	res, err := ProcessImage(bytes.NewReader(testPNG(t, 4000, 1000)))
	if err != nil {
		t.Fatal(err)
	}
	if res.Width != 2048 || res.Height != 512 {
		t.Fatalf("got %dx%d, want 2048x512", res.Width, res.Height)
	}
	full, _ := jpeg.DecodeConfig(bytes.NewReader(res.Full))
	if full.Width != 2048 || full.Height != 512 {
		t.Fatalf("full jpeg %dx%d", full.Width, full.Height)
	}
	thumb, _ := jpeg.DecodeConfig(bytes.NewReader(res.Thumb))
	if thumb.Width != 480 || thumb.Height != 120 {
		t.Fatalf("thumb %dx%d, want 480x120", thumb.Width, thumb.Height)
	}

	res, err = ProcessImage(bytes.NewReader(testPNG(t, 100, 50)))
	if err != nil {
		t.Fatal(err)
	}
	if res.Width != 100 || res.Height != 50 {
		t.Fatalf("got %dx%d, want 100x50 (no upscale)", res.Width, res.Height)
	}
	thumb, _ = jpeg.DecodeConfig(bytes.NewReader(res.Thumb))
	if thumb.Width != 100 || thumb.Height != 50 {
		t.Fatalf("thumb %dx%d, want 100x50", thumb.Width, thumb.Height)
	}
}

func TestProcessImage_RejectsHugeDimensions(t *testing.T) {
	var buf bytes.Buffer
	buf.Write([]byte("\x89PNG\r\n\x1a\n"))
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], 20000)
	binary.BigEndian.PutUint32(ihdr[4:], 20000)
	ihdr[8], ihdr[9] = 8, 0 // 8-bit grayscale
	chunk := append([]byte("IHDR"), ihdr...)
	_ = binary.Write(&buf, binary.BigEndian, uint32(13))
	buf.Write(chunk)
	_ = binary.Write(&buf, binary.BigEndian, crc32.ChecksumIEEE(chunk))

	_, err := ProcessImage(bytes.NewReader(buf.Bytes()))
	if !errors.Is(err, ErrImageTooLarge) {
		t.Fatalf("got %v, want ErrImageTooLarge", err)
	}
}

func TestProcessImage_RejectsNonImage(t *testing.T) {
	var g bytes.Buffer
	if err := gif.Encode(&g, image.NewPaletted(image.Rect(0, 0, 1, 1), color.Palette{color.Black}), nil); err != nil {
		t.Fatal(err)
	}
	cases := map[string][]byte{
		"html":  []byte("<html><body>hi</body></html>"),
		"gif":   g.Bytes(),
		"empty": {},
	}
	for name, data := range cases {
		if _, err := ProcessImage(bytes.NewReader(data)); !errors.Is(err, ErrUnsupportedFormat) {
			t.Errorf("%s: got %v, want ErrUnsupportedFormat", name, err)
		}
	}
}

func TestProcessImage_AcceptsWebP(t *testing.T) {
	// 1x1 lossless WebP.
	data, err := base64.StdEncoding.DecodeString("UklGRhoAAABXRUJQVlA4TA0AAAAvAAAAEAcQERGIiP4HAA==")
	if err != nil {
		t.Fatal(err)
	}
	res, err := ProcessImage(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if res.Width != 1 || res.Height != 1 {
		t.Fatalf("got %dx%d", res.Width, res.Height)
	}
	if _, err := jpeg.Decode(bytes.NewReader(res.Full)); err != nil {
		t.Fatal(err)
	}
}
