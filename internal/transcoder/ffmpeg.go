package transcoder

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// FFmpegConfig holds configuration for the FFmpeg transcoder.
type FFmpegConfig struct {
	// FFmpegPath is the path to the ffmpeg binary.
	// If empty, "ffmpeg" will be used (assumes it's in PATH).
	FFmpegPath string

	// VideoHeight is the target video height in pixels.
	// Width is calculated automatically to maintain aspect ratio.
	// Default: 720
	VideoHeight int

	// VideoCodec is the video codec to use.
	// Default: libx264
	VideoCodec string

	// VideoPreset controls the encoding speed/quality tradeoff.
	// Options: ultrafast, superfast, veryfast, faster, fast, medium, slow, slower, veryslow
	// Default: fast
	VideoPreset string

	// AudioCodec is the audio codec to use.
	// Default: aac
	AudioCodec string

	// HLSSegmentDuration is the target duration of each HLS segment in seconds.
	// Default: 6 (Apple recommended)
	HLSSegmentDuration int

	// HLSPlaylistType sets the playlist type.
	// Use "vod" for Video on Demand (adds EXT-X-ENDLIST tag).
	// Default: vod
	HLSPlaylistType string
}

// DefaultFFmpegConfig returns an FFmpegConfig with production-ready defaults.
func DefaultFFmpegConfig() FFmpegConfig {
	return FFmpegConfig{
		FFmpegPath:         "ffmpeg",
		VideoHeight:        720,
		VideoCodec:         "libx264",
		VideoPreset:        "fast",
		AudioCodec:         "aac",
		HLSSegmentDuration: 6,
		HLSPlaylistType:    "vod",
	}
}

// FFmpegTranscoder implements Transcoder using FFmpeg CLI.
type FFmpegTranscoder struct {
	config FFmpegConfig
}

// Compile-time verification that FFmpegTranscoder implements Transcoder.
var _ Transcoder = (*FFmpegTranscoder)(nil)

// NewFFmpegTranscoder creates a new FFmpeg-based transcoder.
func NewFFmpegTranscoder(cfg FFmpegConfig) *FFmpegTranscoder {
	return &FFmpegTranscoder{
		config: cfg,
	}
}

// TranscodeToHLS converts the input video to HLS format using FFmpeg.
// It executes FFmpeg as a subprocess and waits for completion.
func (t *FFmpegTranscoder) TranscodeToHLS(ctx context.Context, inputPath, outputDir string) (*HLSOutput, error) {
	if err := t.validateInput(inputPath); err != nil {
		return nil, err
	}

	if err := t.validateOutputDir(outputDir); err != nil {
		return nil, err
	}

	manifestPath := filepath.Join(outputDir, "playlist.m3u8")
	segmentPattern := filepath.Join(outputDir, "segment_%03d.ts")

	args := t.buildFFmpegArgs(inputPath, manifestPath, segmentPattern)

	cmd := exec.CommandContext(ctx, t.config.FFmpegPath, args...)
	cmd.Stdout = nil // Discard stdout
	cmd.Stderr = nil // Discard stderr (FFmpeg outputs progress to stderr)

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("transcoding cancelled: %w", ctx.Err())
		}
		return nil, fmt.Errorf("ffmpeg execution failed: %w", err)
	}

	segments, err := t.collectSegments(outputDir)
	if err != nil {
		return nil, fmt.Errorf("failed to collect segments: %w", err)
	}

	return &HLSOutput{
		ManifestPath: manifestPath,
		SegmentPaths: segments,
	}, nil
}

// validateInput checks if the input file exists and is readable.
func (t *FFmpegTranscoder) validateInput(inputPath string) error {
	info, err := os.Stat(inputPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("input file does not exist: %s", inputPath)
		}
		return fmt.Errorf("failed to access input file: %w", err)
	}

	if info.IsDir() {
		return fmt.Errorf("input path is a directory, expected a file: %s", inputPath)
	}

	return nil
}

// validateOutputDir checks if the output directory exists.
func (t *FFmpegTranscoder) validateOutputDir(outputDir string) error {
	info, err := os.Stat(outputDir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("output directory does not exist: %s", outputDir)
		}
		return fmt.Errorf("failed to access output directory: %w", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("output path is not a directory: %s", outputDir)
	}

	return nil
}

// buildFFmpegArgs constructs the FFmpeg command arguments.
func (t *FFmpegTranscoder) buildFFmpegArgs(inputPath, manifestPath, segmentPattern string) []string {
	// Scale filter: -2 ensures width is divisible by 2 (required by many codecs)
	scaleFilter := fmt.Sprintf("scale=-2:%d", t.config.VideoHeight)

	return []string{
		"-i", inputPath,
		"-vf", scaleFilter,
		"-c:v", t.config.VideoCodec,
		"-preset", t.config.VideoPreset,
		"-c:a", t.config.AudioCodec,
		"-f", "hls",
		"-hls_time", fmt.Sprintf("%d", t.config.HLSSegmentDuration),
		"-hls_list_size", "0", // Include all segments in playlist
		"-hls_playlist_type", t.config.HLSPlaylistType,
		"-hls_segment_filename", segmentPattern,
		"-y", // Overwrite output files without asking
		manifestPath,
	}
}

// collectSegments finds all generated .ts segment files in the output directory.
func (t *FFmpegTranscoder) collectSegments(outputDir string) ([]string, error) {
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read output directory: %w", err)
	}

	var segments []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".ts") {
			segments = append(segments, filepath.Join(outputDir, entry.Name()))
		}
	}

	if len(segments) == 0 {
		return nil, fmt.Errorf("no segments generated in output directory")
	}

	return segments, nil
}
