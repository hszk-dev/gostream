package transcoder

import (
	"context"
)

// HLSOutput contains the result of an HLS transcoding operation.
type HLSOutput struct {
	// ManifestPath is the path to the generated .m3u8 manifest file.
	ManifestPath string
	// SegmentPaths contains paths to all generated .ts segment files.
	SegmentPaths []string
}

// Transcoder defines the interface for video transcoding operations.
// Implementations should handle the conversion of video files to streaming formats.
type Transcoder interface {
	// TranscodeToHLS converts an input video file to HLS format.
	// It generates a .m3u8 manifest and .ts segment files in the specified output directory.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout control
	//   - inputPath: Absolute path to the source video file
	//   - outputDir: Directory where HLS files will be generated
	//
	// Returns:
	//   - HLSOutput containing paths to generated manifest and segments
	//   - error if transcoding fails
	//
	// The output directory must exist before calling this method.
	TranscodeToHLS(ctx context.Context, inputPath, outputDir string) (*HLSOutput, error)
}
