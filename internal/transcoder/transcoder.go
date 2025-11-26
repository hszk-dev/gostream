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

// Variant represents a single quality level for ABR (Adaptive Bitrate) streaming.
type Variant struct {
	// Name is the identifier for this variant (e.g., "1080p", "720p", "360p").
	Name string
	// Height is the video height in pixels. Width is calculated to maintain aspect ratio.
	Height int
	// Bitrate is the target bitrate in bits per second, used in master playlist.
	Bitrate int
}

// VariantOutput contains the result for a single quality variant.
type VariantOutput struct {
	// Variant is the configuration used for this output.
	Variant Variant
	// ManifestPath is the path to the variant's playlist.m3u8 file.
	ManifestPath string
	// SegmentPaths contains paths to all .ts segment files for this variant.
	SegmentPaths []string
}

// ABROutput contains the result of a multi-bitrate transcoding operation.
type ABROutput struct {
	// MasterManifestPath is the path to the generated master.m3u8 file.
	MasterManifestPath string
	// Variants contains output information for each quality level.
	Variants []VariantOutput
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

	// TranscodeToABR converts an input video file to multiple quality variants for ABR streaming.
	// It generates a master.m3u8 and subdirectories containing variant playlists and segments.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout control
	//   - inputPath: Absolute path to the source video file
	//   - outputDir: Directory where HLS files will be generated
	//   - variants: Quality variants to generate (e.g., 1080p, 720p, 360p)
	//
	// Returns:
	//   - ABROutput containing paths to master manifest and all variant outputs
	//   - error if transcoding fails
	//
	// The output directory must exist before calling this method.
	// Each variant will be placed in a subdirectory named after the variant (e.g., outputDir/720p/).
	TranscodeToABR(ctx context.Context, inputPath, outputDir string, variants []Variant) (*ABROutput, error)
}
