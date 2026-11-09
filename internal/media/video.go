package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

const (
	maxVideoSeconds = 60.5
	ffmpegTimeout   = 2 * time.Minute
)

// VideoResult describes a remuxed, metadata-free MP4 and its thumbnail.
type VideoResult struct {
	Path          string
	Thumb         []byte
	Width, Height int
	DurationMs    int64
}

type VideoProcessor interface {
	Process(ctx context.Context, inPath, outDir string) (*VideoResult, error)
}

// FFmpeg implements VideoProcessor using ffmpeg/ffprobe from PATH.
type FFmpeg struct{}

func NewFFmpeg() *FFmpeg { return &FFmpeg{} }

func (f *FFmpeg) Available() bool {
	_, err1 := exec.LookPath("ffmpeg")
	_, err2 := exec.LookPath("ffprobe")
	return err1 == nil && err2 == nil
}

type probeOutput struct {
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
	Streams []struct {
		CodecType string `json:"codec_type"`
		CodecName string `json:"codec_name"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
		Duration  string `json:"duration"`
	} `json:"streams"`
}

func (f *FFmpeg) Process(ctx context.Context, inPath, outDir string) (*VideoResult, error) {
	ctx, cancel := context.WithTimeout(ctx, ffmpegTimeout)
	defer cancel()

	probeJSON, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-print_format", "json",
		"-show_format", "-show_streams", inPath).Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: ffprobe failed", ErrUnsupportedFormat)
	}
	var probe probeOutput
	if err := json.Unmarshal(probeJSON, &probe); err != nil {
		return nil, fmt.Errorf("%w: unreadable probe output", ErrUnsupportedFormat)
	}

	videoIdx := -1
	for i, s := range probe.Streams {
		if s.CodecType == "video" {
			videoIdx = i
			break
		}
	}
	if videoIdx < 0 {
		return nil, fmt.Errorf("%w: no video stream", ErrUnsupportedFormat)
	}
	vs := probe.Streams[videoIdx]

	durStr := probe.Format.Duration
	if durStr == "" {
		durStr = vs.Duration
	}
	seconds, err := strconv.ParseFloat(durStr, 64)
	if err != nil || seconds <= 0 {
		return nil, fmt.Errorf("%w: unknown duration", ErrUnsupportedFormat)
	}
	if seconds > maxVideoSeconds {
		return nil, ErrVideoTooLong
	}

	outPath := filepath.Join(outDir, "video.mp4")
	args := []string{"-y", "-v", "error", "-i", inPath,
		"-map", "0:v:0", "-map", "0:a:0?",
		"-map_metadata", "-1", "-map_metadata:s", "-1", "-map_chapters", "-1",
		"-fflags", "+bitexact", "-flags:v", "+bitexact", "-flags:a", "+bitexact",
		"-c", "copy", "-movflags", "+faststart"}
	if vs.CodecName == "hevc" {
		args = append(args, "-tag:v", "hvc1")
	}
	args = append(args, outPath)
	if out, err := run(ctx, "ffmpeg", args...); err != nil {
		return nil, fmt.Errorf("%w: remux failed: %s", ErrUnsupportedFormat, out)
	}

	thumbPath := filepath.Join(outDir, "thumb.jpg")
	if out, err := run(ctx, "ffmpeg", "-y", "-v", "error", "-i", outPath, "-frames:v", "1",
		"-vf", "scale='if(gt(iw,ih),480,-2)':'if(gt(iw,ih),-2,480)'",
		"-map_metadata", "-1", "-f", "image2", "-c:v", "mjpeg", thumbPath); err != nil {
		return nil, fmt.Errorf("thumbnail failed: %w: %s", err, out)
	}
	thumb, err := os.ReadFile(thumbPath)
	if err != nil {
		return nil, err
	}
	_ = os.Remove(thumbPath)

	return &VideoResult{
		Path:       outPath,
		Thumb:      thumb,
		Width:      vs.Width,
		Height:     vs.Height,
		DurationMs: int64(seconds*1000 + 0.5),
	}, nil
}

func run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var buf bytes.Buffer
	cmd.Stderr = &buf
	cmd.Stdout = &buf
	err := cmd.Run()
	return buf.String(), err
}
