package media

import (
	"context"
	"encoding/json"
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

// ffmpegRun runs ffmpeg for test setup and fails the test on error.
func ffmpegRun(t *testing.T, args ...string) {
	t.Helper()
	if out, err := exec.Command("ffmpeg", append([]string{"-y", "-v", "error"}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg %v: %v\n%s", args, err, out)
	}
}

func requireX264(t *testing.T) {
	t.Helper()
	out, _ := exec.Command("ffmpeg", "-hide_banner", "-encoders").Output()
	if !strings.Contains(string(out), "libx264") {
		t.Skip("libx264 not available (h264 is the only allowed video codec)")
	}
}

func makeTestVideo(t *testing.T, path string, seconds string, extra ...string) {
	t.Helper()
	makeTestVideoWithInputs(t, path, seconds, nil, extra...)
}

// makeTestVideoWithInputs adds extra input args (e.g. "-i", "subs.srt") after the two lavfi inputs.
func makeTestVideoWithInputs(t *testing.T, path, seconds string, inputs []string, extra ...string) {
	t.Helper()
	requireX264(t)
	args := []string{"-f", "lavfi", "-i", "testsrc=duration=" + seconds + ":size=320x240:rate=10",
		"-f", "lavfi", "-i", "sine=duration=" + seconds}
	args = append(args, inputs...)
	args = append(args, "-metadata", "location=+13.7563+100.5018/", "-metadata:s:v", "title=SECRET-GPS-13.7563",
		"-c:v", "libx264", "-c:a", "aac")
	ffmpegRun(t, append(append(args, extra...), path)...)
}

func probe(t *testing.T, path string, args ...string) string {
	t.Helper()
	out, err := exec.Command("ffprobe", append([]string{"-v", "error"}, append(args, path)...)...).CombinedOutput()
	if err != nil {
		t.Fatal(err, string(out))
	}
	return string(out)
}

func codecTypes(t *testing.T, path string) []string {
	t.Helper()
	out := probe(t, path, "-show_entries", "stream=codec_type", "-of", "csv=p=0")
	return strings.Fields(out)
}

func assertNoLocation(t *testing.T, path string) {
	t.Helper()
	out := probe(t, path, "-show_format", "-show_streams")
	if strings.Contains(out, "13.7563") || strings.Contains(out, "SECRET") {
		t.Errorf("output still has GPS/metadata:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		// Only tag lines count: ffprobe also prints "chroma_location" as a plain stream field.
		if strings.HasPrefix(line, "TAG:") && strings.Contains(strings.ToLower(line), "location") {
			t.Errorf("output still has location tag: %s", line)
		}
	}
}

func TestFFmpeg_StripsLocation(t *testing.T) {
	f := requireFFmpeg(t)
	dir := t.TempDir()
	in := filepath.Join(dir, "in.mp4")
	makeTestVideo(t, in, "2", "-movflags", "use_metadata_tags")
	// positive control: the input really carries the secret
	if !strings.Contains(probe(t, in, "-show_format", "-show_streams"), "13.7563") {
		t.Fatal("test input does not contain GPS metadata; test would prove nothing")
	}
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
	assertNoLocation(t, res.Path)
}

func TestFFmpeg_DropsExtraStreams(t *testing.T) {
	f := requireFFmpeg(t)
	dir := t.TempDir()
	srt := filepath.Join(dir, "s.srt")
	if err := os.WriteFile(srt, []byte("1\n00:00:00,000 --> 00:00:01,000\nhello 13.7563\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	in := filepath.Join(dir, "in.mp4")
	makeTestVideoWithInputs(t, in, "2", []string{"-i", srt}, "-map", "0", "-map", "1", "-map", "2", "-c:s", "mov_text")
	if n := len(codecTypes(t, in)); n < 3 {
		t.Fatalf("input has %d streams, want >2", n)
	}
	outDir := t.TempDir()
	res, err := f.Process(context.Background(), in, outDir)
	if err != nil {
		t.Fatal(err)
	}
	got := codecTypes(t, res.Path)
	if len(got) != 2 || got[0] != "video" || got[1] != "audio" {
		t.Fatalf("output streams = %v, want [video audio]", got)
	}
}

func TestFFmpeg_ReportsDisplayRotation(t *testing.T) {
	f := requireFFmpeg(t)
	dir := t.TempDir()
	base := filepath.Join(dir, "base.mp4")
	makeTestVideo(t, base, "1")
	rot := filepath.Join(dir, "rot.mp4")
	ffmpegRun(t, "-display_rotation", "90", "-i", base, "-c", "copy", rot)
	res, err := f.Process(context.Background(), rot, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if res.Width != 240 || res.Height != 320 {
		t.Fatalf("size = %dx%d, want 240x320", res.Width, res.Height)
	}
}

func TestFFmpeg_RejectsMJPEGAndConcat(t *testing.T) {
	f := requireFFmpeg(t)
	dir := t.TempDir()
	mj := filepath.Join(dir, "mj.mov")
	ffmpegRun(t, "-f", "lavfi", "-i", "testsrc=duration=1:size=64x64:rate=5", "-c:v", "mjpeg", mj)
	if _, err := f.Process(context.Background(), mj, t.TempDir()); !errors.Is(err, ErrUnsupportedFormat) {
		t.Errorf("mjpeg: got %v, want ErrUnsupportedFormat", err)
	}

	other := filepath.Join(dir, "other.mp4")
	makeTestVideo(t, other, "1")
	cc := filepath.Join(dir, "list.txt")
	if err := os.WriteFile(cc, []byte("ffconcat version 1.0\nfile "+other+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outDir := t.TempDir()
	if _, err := f.Process(context.Background(), cc, outDir); !errors.Is(err, ErrUnsupportedFormat) {
		t.Errorf("ffconcat: got %v, want ErrUnsupportedFormat", err)
	}
	if entries, _ := os.ReadDir(outDir); len(entries) != 0 {
		t.Errorf("outDir not clean after failure: %v", entries)
	}
}

func TestDisplayRotationParsing(t *testing.T) {
	var p probeOutput
	js := `{"streams":[{"codec_type":"video","width":1920,"height":1080,"side_data_list":[{"side_data_type":"Display Matrix","rotation":-90}]}]}`
	if err := json.Unmarshal([]byte(js), &p); err != nil {
		t.Fatal(err)
	}
	s := p.Streams[0]
	rot := displayRotation(s.SideDataList, s.Tags.Rotate)
	if w, h := displaySize(s.Width, s.Height, rot); w != 1080 || h != 1920 {
		t.Fatalf("-90: got %dx%d", w, h)
	}
	if w, h := displaySize(1920, 1080, displayRotation(nil, "270")); w != 1080 || h != 1920 {
		t.Fatalf("tag 270: got %dx%d", w, h)
	}
	if w, h := displaySize(1920, 1080, displayRotation(nil, "180")); w != 1920 || h != 1080 {
		t.Fatalf("180: got %dx%d", w, h)
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
