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

func TestProcessImage_AppliesOrientation(t *testing.T) {
	res, err := ProcessImage(bytes.NewReader(withExif(testJPEG(t, 64, 32), 6, false)))
	if err != nil {
		t.Fatal(err)
	}
	if res.Width != 32 || res.Height != 64 {
		t.Fatalf("got %dx%d, want 32x64", res.Width, res.Height)
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
