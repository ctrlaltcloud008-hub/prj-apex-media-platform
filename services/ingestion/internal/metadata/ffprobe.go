package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"strings"
	"time"
)

type VideoMetadata struct {
	DurationMs int64
	Width      int
	Height     int
	Codec      string
	FPS        float64
	BitrateBps int64
	IsHDR      bool
}

type ffprobeOutput struct {
	Streams []ffprobeStream `json:"streams"`
	Format  ffprobeFormat   `json:"format"`
}

type ffprobeStream struct {
	CodecType string `json:"codec_type"`
	CodecName string `json:"codec_name"`

	Width  int `json:"width,omitempty"`
	Height int `json:"height,omitempty"`

	RFrameRate     string `json:"r_frame_rate,omitempty"`
	ColorTransfer  string `json:"color_transfer,omitempty"`
	ColorPrimaries string `json:"color_primaries,omitempty"`
	ColorSpace     string `json:"color_space,omitempty"`
}

type ffprobeFormat struct {
	Duration string `json:"duration"`
	BitRate  string `json:"bit_rate"`
}

func ExtractMetadata(ctx context.Context, signedURL string) (*VideoMetadata, error) {

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "quiet", "-print_format", "json", "-show_streams", "-show_format", signedURL)

	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("ffprobe timed out: %w", ctx.Err())
		}
		return nil, fmt.Errorf("ffprobe failed: %w", err)
	}

	return parseFFprobeOutput(out)
}

func parseFFprobeOutput(data []byte) (*VideoMetadata, error) {
	var output ffprobeOutput
	if err := json.Unmarshal(data, &output); err != nil {
		return nil, fmt.Errorf("failed to parse ffprobe output: %w", err)
	}

	var video *ffprobeStream
	for i := range output.Streams {
		if output.Streams[i].CodecType == "video" {
			video = &output.Streams[i]
			break
		}
	}

	if video == nil {
		return nil, fmt.Errorf("no video stream found")
	}

	fps, err := parseFraction(video.RFrameRate)
	if err != nil {
		return nil, fmt.Errorf("invalid frame rate %q: %w", video.RFrameRate, err)
	}

	durationMs, err := parseDurationMs(output.Format.Duration)
	if err != nil {
		return nil, fmt.Errorf("invalid duration %q: %w", output.Format.Duration, err)
	}

	var bitrateBps int64
	if output.Format.BitRate != "" {
		fmt.Sscanf(output.Format.BitRate, "%d", &bitrateBps)
	}

	return &VideoMetadata{
		DurationMs: durationMs,
		Width:      video.Width,
		Height:     video.Height,
		Codec:      video.CodecName,
		FPS:        fps,
		BitrateBps: bitrateBps,
		IsHDR:      isHDR(video),
	}, nil
}

func isHDR(s *ffprobeStream) bool {
	switch s.ColorTransfer {
	case "smpte2084", "arib-std-b67":
		return true
	}

	if s.ColorPrimaries == "bt2020" && s.ColorSpace == "bt2020nc" {
		return true
	}
	return false
}

func parseFraction(s string) (float64, error) {
	parts := strings.Split(s, "/")
	if len(parts) != 2 {
		var f float64
		_, err := fmt.Sscanf(s, "%f", &f)
		return f, err
	}

	var num, den float64
	fmt.Sscanf(parts[0], "%f", &num)
	fmt.Sscanf(parts[1], "%f", &den)

	if den == 0 {
		return 0, fmt.Errorf("denominator is zero")
	}

	return math.Round(num/den*100) / 100, nil
}

func parseDurationMs(s string) (int64, error) {
	var secs float64
	_, err := fmt.Sscanf(s, "%f", &secs)
	if err != nil {
		return 0, err
	}
	return int64(secs * 1000), nil
}
