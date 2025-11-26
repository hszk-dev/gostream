package transcoder

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultFFmpegConfig(t *testing.T) {
	cfg := DefaultFFmpegConfig()

	tests := []struct {
		name     string
		got      any
		expected any
	}{
		{"FFmpegPath", cfg.FFmpegPath, "ffmpeg"},
		{"VideoHeight", cfg.VideoHeight, 720},
		{"VideoCodec", cfg.VideoCodec, "libx264"},
		{"VideoPreset", cfg.VideoPreset, "fast"},
		{"AudioCodec", cfg.AudioCodec, "aac"},
		{"HLSSegmentDuration", cfg.HLSSegmentDuration, 6},
		{"HLSPlaylistType", cfg.HLSPlaylistType, "vod"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("got %v, expected %v", tt.got, tt.expected)
			}
		})
	}
}

func TestFFmpegTranscoder_ValidateInput(t *testing.T) {
	transcoder := NewFFmpegTranscoder(DefaultFFmpegConfig())

	t.Run("non-existent file returns error", func(t *testing.T) {
		err := transcoder.validateInput("/non/existent/file.mp4")
		if err == nil {
			t.Error("expected error for non-existent file")
		}
	})

	t.Run("directory returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		err := transcoder.validateInput(tmpDir)
		if err == nil {
			t.Error("expected error when input is a directory")
		}
	})

	t.Run("existing file succeeds", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "test.mp4")
		if err := os.WriteFile(tmpFile, []byte("dummy"), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		err := transcoder.validateInput(tmpFile)
		if err != nil {
			t.Errorf("unexpected error for existing file: %v", err)
		}
	})
}

func TestFFmpegTranscoder_ValidateOutputDir(t *testing.T) {
	transcoder := NewFFmpegTranscoder(DefaultFFmpegConfig())

	t.Run("non-existent directory returns error", func(t *testing.T) {
		err := transcoder.validateOutputDir("/non/existent/dir")
		if err == nil {
			t.Error("expected error for non-existent directory")
		}
	})

	t.Run("file instead of directory returns error", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "file.txt")
		if err := os.WriteFile(tmpFile, []byte("dummy"), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		err := transcoder.validateOutputDir(tmpFile)
		if err == nil {
			t.Error("expected error when output is a file")
		}
	})

	t.Run("existing directory succeeds", func(t *testing.T) {
		tmpDir := t.TempDir()
		err := transcoder.validateOutputDir(tmpDir)
		if err != nil {
			t.Errorf("unexpected error for existing directory: %v", err)
		}
	})
}

func TestFFmpegTranscoder_BuildFFmpegArgs(t *testing.T) {
	cfg := DefaultFFmpegConfig()
	transcoder := NewFFmpegTranscoder(cfg)

	inputPath := "/input/video.mp4"
	manifestPath := "/output/playlist.m3u8"
	segmentPattern := "/output/segment_%03d.ts"

	args := transcoder.buildFFmpegArgs(inputPath, manifestPath, segmentPattern)

	expectedArgs := []string{
		"-i", "/input/video.mp4",
		"-vf", "scale=-2:720",
		"-c:v", "libx264",
		"-preset", "fast",
		"-c:a", "aac",
		"-f", "hls",
		"-hls_time", "6",
		"-hls_list_size", "0",
		"-hls_playlist_type", "vod",
		"-hls_segment_filename", "/output/segment_%03d.ts",
		"-y",
		"/output/playlist.m3u8",
	}

	if len(args) != len(expectedArgs) {
		t.Fatalf("arg count mismatch: got %d, expected %d", len(args), len(expectedArgs))
	}

	for i, expected := range expectedArgs {
		if args[i] != expected {
			t.Errorf("arg[%d]: got %q, expected %q", i, args[i], expected)
		}
	}
}

func TestFFmpegTranscoder_BuildFFmpegArgs_CustomConfig(t *testing.T) {
	cfg := FFmpegConfig{
		FFmpegPath:         "/usr/local/bin/ffmpeg",
		VideoHeight:        1080,
		VideoCodec:         "libx265",
		VideoPreset:        "slow",
		AudioCodec:         "opus",
		HLSSegmentDuration: 10,
		HLSPlaylistType:    "event",
	}
	transcoder := NewFFmpegTranscoder(cfg)

	args := transcoder.buildFFmpegArgs("/in.mp4", "/out/playlist.m3u8", "/out/seg_%03d.ts")

	// Verify custom values are used
	tests := []struct {
		name     string
		argIndex int
		expected string
	}{
		{"scale filter uses custom height", 3, "scale=-2:1080"},
		{"video codec", 5, "libx265"},
		{"preset", 7, "slow"},
		{"audio codec", 9, "opus"},
		{"hls_time", 13, "10"},
		{"hls_playlist_type", 17, "event"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if args[tt.argIndex] != tt.expected {
				t.Errorf("got %q, expected %q", args[tt.argIndex], tt.expected)
			}
		})
	}
}

func TestFFmpegTranscoder_CollectSegments(t *testing.T) {
	transcoder := NewFFmpegTranscoder(DefaultFFmpegConfig())

	t.Run("collects ts files", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create mock segment files
		segmentFiles := []string{"segment_000.ts", "segment_001.ts", "segment_002.ts"}
		for _, name := range segmentFiles {
			path := filepath.Join(tmpDir, name)
			if err := os.WriteFile(path, []byte("dummy"), 0644); err != nil {
				t.Fatalf("failed to create segment file: %v", err)
			}
		}

		// Create non-segment files that should be ignored
		os.WriteFile(filepath.Join(tmpDir, "playlist.m3u8"), []byte("dummy"), 0644)
		os.WriteFile(filepath.Join(tmpDir, "other.txt"), []byte("dummy"), 0644)

		segments, err := transcoder.collectSegments(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(segments) != 3 {
			t.Errorf("expected 3 segments, got %d", len(segments))
		}
	})

	t.Run("returns error when no segments found", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create only non-ts files
		os.WriteFile(filepath.Join(tmpDir, "playlist.m3u8"), []byte("dummy"), 0644)

		_, err := transcoder.collectSegments(tmpDir)
		if err == nil {
			t.Error("expected error when no segments found")
		}
	})

	t.Run("ignores subdirectories", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create a segment file
		os.WriteFile(filepath.Join(tmpDir, "segment_000.ts"), []byte("dummy"), 0644)

		// Create a subdirectory (should be ignored)
		os.Mkdir(filepath.Join(tmpDir, "subdir"), 0755)

		segments, err := transcoder.collectSegments(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(segments) != 1 {
			t.Errorf("expected 1 segment, got %d", len(segments))
		}
	})
}

func TestFFmpegTranscoder_TranscodeToHLS_ValidationErrors(t *testing.T) {
	transcoder := NewFFmpegTranscoder(DefaultFFmpegConfig())
	ctx := context.Background()

	t.Run("returns error for non-existent input", func(t *testing.T) {
		outputDir := t.TempDir()
		_, err := transcoder.TranscodeToHLS(ctx, "/non/existent/input.mp4", outputDir)
		if err == nil {
			t.Error("expected error for non-existent input")
		}
	})

	t.Run("returns error for non-existent output directory", func(t *testing.T) {
		// Create a temporary input file
		inputFile := filepath.Join(t.TempDir(), "input.mp4")
		os.WriteFile(inputFile, []byte("dummy"), 0644)

		_, err := transcoder.TranscodeToHLS(ctx, inputFile, "/non/existent/output")
		if err == nil {
			t.Error("expected error for non-existent output directory")
		}
	})
}

func TestFFmpegTranscoder_TranscodeToHLS_ContextCancellation(t *testing.T) {
	// Use a non-existent ffmpeg path to make the command fail
	cfg := DefaultFFmpegConfig()
	cfg.FFmpegPath = "/non/existent/ffmpeg"
	transcoder := NewFFmpegTranscoder(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	inputFile := filepath.Join(t.TempDir(), "input.mp4")
	os.WriteFile(inputFile, []byte("dummy"), 0644)
	outputDir := t.TempDir()

	_, err := transcoder.TranscodeToHLS(ctx, inputFile, outputDir)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}
