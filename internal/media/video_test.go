package media

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func requireFFmpeg(t *testing.T) *FFmpeg {
	t.Helper()
	f := NewFFmpeg()
	if !f.Available() {
		t.Skip("ffmpeg/ffprobe not available")
	}
	return f
}

func makeTestVideo(t *testing.T, path string, seconds string) {
	t.Helper()
	for _, vcodec := range []string{"libx264", "mpeg4"} {
		cmd := exec.Command("ffmpeg", "-y", "-f", "lavfi", "-i", "testsrc=duration="+seconds+":size=320x240:rate=10",
			"-f", "lavfi", "-i", "sine=duration="+seconds,
			"-metadata", "location=+13.7563+100.5018/", "-c:v", vcodec, "-c:a", "aac", path)
		if out, err := cmd.CombinedOutput(); err == nil {
			return
		} else if vcodec == "mpeg4" {
			t.Fatalf("cannot create test video: %v\n%s", err, out)
		}
	}
}

func TestFFmpeg_StripsLocation(t *testing.T) {
	f := requireFFmpeg(t)
	dir := t.TempDir()
	in := filepath.Join(dir, "in.mp4")
	makeTestVideo(t, in, "2")
	outDir := filepath.Join(dir, "out")
	if err := os.Mkdir(outDir, 0o755); err != nil {
		t.Fatal(err)
	}

	res, err := f.Process(context.Background(), in, outDir)
	if err != nil {
		t.Fatal(err)
	}
	if res.DurationMs < 1700 || res.DurationMs > 2300 {
		t.Errorf("DurationMs = %d", res.DurationMs)
	}
	if res.Width != 320 || res.Height != 240 {
		t.Errorf("size = %dx%d", res.Width, res.Height)
	}
	if len(res.Thumb) < 2 || res.Thumb[0] != 0xFF || res.Thumb[1] != 0xD8 {
		t.Error("thumb is not a JPEG")
	}
	out, err := exec.Command("ffprobe", "-v", "error", "-show_format", "-show_streams", res.Path).CombinedOutput()
	if err != nil {
		t.Fatal(err, string(out))
	}
	if strings.Contains(string(out), "13.7563") {
		t.Errorf("output still has GPS coordinates:\n%s", out)
	}
	for _, line := range strings.Split(string(out), "\n") {
		// Only tag lines count: ffprobe also prints "chroma_location" as a plain stream field.
		if strings.HasPrefix(line, "TAG:") && strings.Contains(strings.ToLower(line), "location") {
			t.Errorf("output still has location tag: %s", line)
		}
	}
}

func TestFFmpeg_RejectsLong(t *testing.T) {
	f := requireFFmpeg(t)
	dir := t.TempDir()
	in := filepath.Join(dir, "long.mp4")
	makeTestVideo(t, in, "61")
	_, err := f.Process(context.Background(), in, dir)
	if !errors.Is(err, ErrVideoTooLong) {
		t.Fatalf("got %v, want ErrVideoTooLong", err)
	}
}

func TestFFmpeg_RejectsNonVideo(t *testing.T) {
	f := requireFFmpeg(t)
	dir := t.TempDir()
	in := filepath.Join(dir, "x.mp4")
	if err := os.WriteFile(in, []byte("just some text"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := f.Process(context.Background(), in, dir)
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("got %v, want ErrUnsupportedFormat", err)
	}
}
