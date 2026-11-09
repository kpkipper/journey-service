package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
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
		Tags      struct {
			Rotate string `json:"rotate"`
		} `json:"tags"`
		SideDataList []struct {
			Rotation *float64 `json:"rotation"`
		} `json:"side_data_list"`
	} `json:"streams"`
}

var (
	allowedVideoCodecs = map[string]bool{"h264": true, "hevc": true}
	allowedAudioCodecs = map[string]bool{"aac": true, "mp3": true, "alac": true, "ac3": true, "eac3": true}
)

// Only local files, and only the MOV/MP4 demuxer: no auto-detected playlist/concat inputs.
var inputFlags = []string{"-protocol_whitelist", "file", "-f", "mov"}

// displayRotation returns the rotation in degrees from display-matrix side data or the legacy rotate tag.
func displayRotation(sideData []struct {
	Rotation *float64 `json:"rotation"`
}, rotateTag string) int {
	for _, sd := range sideData {
		if sd.Rotation != nil {
			return int(math.Round(*sd.Rotation))
		}
	}
	if v, err := strconv.Atoi(rotateTag); err == nil {
		return v
	}
	return 0
}

// displaySize swaps width and height when the video is rotated a quarter turn.
func displaySize(w, h, rotation int) (int, int) {
	if r := ((rotation % 180) + 180) % 180; r == 90 {
		return h, w
	}
	return w, h
}

func (f *FFmpeg) Process(ctx context.Context, inPath, outDir string) (*VideoResult, error) {
	ctx, cancel := context.WithTimeout(ctx, ffmpegTimeout)
	defer cancel()

	probeJSON, err := exec.CommandContext(ctx, "ffprobe", append(append([]string{"-v", "error"}, inputFlags...),
		"-print_format", "json", "-show_format", "-show_streams", inPath)...).Output()
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

	videoIdx, audioIdx := -1, -1
	for i, s := range probe.Streams {
		if s.CodecType == "video" && videoIdx < 0 {
			videoIdx = i
		}
		if s.CodecType == "audio" && audioIdx < 0 {
			audioIdx = i
		}
	}
	if videoIdx < 0 {
		return nil, fmt.Errorf("%w: no video stream", ErrUnsupportedFormat)
	}
	vs := probe.Streams[videoIdx]
	if !allowedVideoCodecs[vs.CodecName] {
		return nil, fmt.Errorf("%w: video codec %q", ErrUnsupportedFormat, vs.CodecName)
	}
	if audioIdx >= 0 && !allowedAudioCodecs[probe.Streams[audioIdx].CodecName] {
		return nil, fmt.Errorf("%w: audio codec %q", ErrUnsupportedFormat, probe.Streams[audioIdx].CodecName)
	}

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
	thumbPath := filepath.Join(outDir, "thumb.jpg")
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(outPath)
			_ = os.Remove(thumbPath)
		}
	}()

	args := append([]string{"-y", "-v", "error"}, inputFlags...)
	args = append(args, "-i", inPath,
		"-map", "0:v:0", "-map", "0:a:0?",
		"-map_metadata", "-1", "-map_metadata:s", "-1", "-map_chapters", "-1",
		"-fflags", "+bitexact", "-flags:v", "+bitexact", "-flags:a", "+bitexact",
		"-c", "copy", "-movflags", "+faststart")
	if vs.CodecName == "hevc" {
		args = append(args, "-tag:v", "hvc1")
	}
	args = append(args, outPath)
	if err := run(ctx, "ffmpeg", args...); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: remux failed: %w", ErrUnsupportedFormat, err)
	}

	if err := run(ctx, "ffmpeg", "-y", "-v", "error", "-i", outPath, "-frames:v", "1",
		"-vf", "scale='if(gt(iw,ih),480,-2)':'if(gt(iw,ih),-2,480)'",
		"-map_metadata", "-1", "-f", "image2", "-c:v", "mjpeg", thumbPath); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("thumbnail failed: %w", err)
	}
	thumb, err := os.ReadFile(thumbPath)
	if err != nil {
		return nil, err
	}
	_ = os.Remove(thumbPath)

	w, h := displaySize(vs.Width, vs.Height, displayRotation(vs.SideDataList, vs.Tags.Rotate))
	ok = true
	return &VideoResult{
		Path:       outPath,
		Thumb:      thumb,
		Width:      w,
		Height:     h,
		DurationMs: int64(seconds*1000 + 0.5),
	}, nil
}

func run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	var buf bytes.Buffer
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		// keep only the last stderr line: enough to log, short enough for an error message
		lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
		return fmt.Errorf("%s: %w: %s", name, err, lines[len(lines)-1])
	}
	return nil
}
