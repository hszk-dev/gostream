# Adaptive Bitrate (ABR) Streaming

## 1. Background & Goals

### Current State
- Single resolution transcoding (720p fixed)
- Single `playlist.m3u8` per video
- `HLSOutput` struct: `ManifestPath` + `SegmentPaths[]`

### Target State
- Multi-resolution transcoding (1080p, 720p, 360p)
- Master playlist (`master.m3u8`) referencing variant playlists
- Each variant has its own playlist and segments

### Why ABR Matters
ABR is essential for production video services because:
1. **Network Adaptation**: Players automatically switch quality based on bandwidth
2. **Reduced Buffering**: Lower quality available when network is poor
3. **Better UX**: Smooth playback across diverse network conditions

---

## 2. Technical Design

### 2.1 Output Structure (After Implementation)

```
hls/{videoID}/
├── master.m3u8           # Master playlist (ABR manifest)
├── 1080p/
│   ├── playlist.m3u8     # Variant playlist for 1080p
│   ├── segment_000.ts
│   ├── segment_001.ts
│   └── ...
├── 720p/
│   ├── playlist.m3u8     # Variant playlist for 720p
│   ├── segment_000.ts
│   └── ...
└── 360p/
    ├── playlist.m3u8     # Variant playlist for 360p
    ├── segment_000.ts
    └── ...
```

### 2.2 Master Playlist Format (master.m3u8)

```m3u8
#EXTM3U
#EXT-X-VERSION:3

#EXT-X-STREAM-INF:BANDWIDTH=5000000,RESOLUTION=1920x1080
1080p/playlist.m3u8

#EXT-X-STREAM-INF:BANDWIDTH=2500000,RESOLUTION=1280x720
720p/playlist.m3u8

#EXT-X-STREAM-INF:BANDWIDTH=800000,RESOLUTION=640x360
360p/playlist.m3u8
```

### 2.3 Implementation Approach Decision

**Option A: FFmpeg filter_complex (Single pass)**
- Pros: Single input read, potentially faster
- Cons: Complex command, all-or-nothing failure

**Option B: Sequential per-resolution (Recommended)**
- Pros: Simple, debuggable, partial retry possible
- Cons: Multiple input reads, longer total time

**Decision: Option B (Sequential)**

Rationale:
1. **YAGNI Principle**: Start simple, optimize when needed
2. **Debuggability**: Easier to identify which resolution failed
3. **Partial Recovery**: Future enhancement to retry only failed resolutions
4. **Code Clarity**: Each transcoding step is independent and testable

---

## 3. Data Structure Changes

### 3.1 New Variant Configuration

```go
// internal/transcoder/transcoder.go

// Variant represents a single quality level for ABR streaming.
type Variant struct {
    Name       string // e.g., "1080p", "720p", "360p"
    Height     int    // Video height in pixels
    Bitrate    int    // Target bitrate in bps (for master playlist)
}

// ABROutput contains the result of a multi-bitrate transcoding operation.
type ABROutput struct {
    // MasterManifestPath is the path to the generated master.m3u8 file.
    MasterManifestPath string
    // Variants contains output information for each quality level.
    Variants []VariantOutput
}

// VariantOutput contains the result for a single quality variant.
type VariantOutput struct {
    Variant      Variant
    ManifestPath string   // Path to variant's playlist.m3u8
    SegmentPaths []string // Paths to .ts segment files
}
```

### 3.2 Extended Transcoder Interface

```go
// internal/transcoder/transcoder.go

type Transcoder interface {
    // Existing method (kept for backward compatibility during transition)
    TranscodeToHLS(ctx context.Context, inputPath, outputDir string) (*HLSOutput, error)

    // New method for ABR transcoding
    TranscodeToABR(ctx context.Context, inputPath, outputDir string, variants []Variant) (*ABROutput, error)
}
```

### 3.3 Default Variants Configuration

```go
// internal/transcoder/ffmpeg.go

// DefaultABRVariants returns the default set of quality variants.
func DefaultABRVariants() []Variant {
    return []Variant{
        {Name: "1080p", Height: 1080, Bitrate: 5000000},  // ~5 Mbps
        {Name: "720p",  Height: 720,  Bitrate: 2500000},  // ~2.5 Mbps
        {Name: "360p",  Height: 360,  Bitrate: 800000},   // ~800 Kbps
    }
}
```

---

## 4. Implementation Steps

### Step 1: Extend Data Structures
**Files to modify:**
- `internal/transcoder/transcoder.go`

**Changes:**
1. Add `Variant`, `ABROutput`, `VariantOutput` structs
2. Add `TranscodeToABR` method to `Transcoder` interface

**Tests:**
- Unit tests for new struct validation

### Step 2: Implement ABR Transcoding in FFmpeg
**Files to modify:**
- `internal/transcoder/ffmpeg.go`
- `internal/transcoder/ffmpeg_test.go`

**Changes:**
1. Add `DefaultABRVariants()` function
2. Implement `TranscodeToABR()` method:
   - Loop through variants, calling FFmpeg for each
   - Create subdirectories per variant
   - Generate master.m3u8 after all variants complete
3. Add helper method `generateMasterPlaylist()`

**Tests:**
- Table-driven tests for various variant configurations
- Test master playlist generation
- Test error handling (partial failures)

### Step 3: Update TranscodeService
**Files to modify:**
- `internal/usecase/transcode_service.go`
- `internal/usecase/transcode_service_test.go`

**Changes:**
1. Switch from `TranscodeToHLS()` to `TranscodeToABR()`
2. Update `uploadHLSFiles()` to `uploadABRFiles()`:
   - Upload master.m3u8
   - Upload each variant's playlist and segments with correct paths
3. Update `markVideoReady()` to store master manifest key

**Storage key changes:**
```
Before: hls/{videoID}/playlist.m3u8
After:  hls/{videoID}/master.m3u8
```

### Step 4: Configuration Support (Optional Enhancement)
**Files to modify:**
- `internal/config/config.go`

**Changes:**
- Add environment variables for customizing variants (optional)
- Default to hardcoded variants if not specified

### Step 5: Update Tests & Integration Testing
**Files to modify:**
- All test files for modified packages

**Manual testing:**
1. Upload a video through API
2. Verify master.m3u8 and all variant directories are created
3. Test playback with HLS-compatible player (e.g., VLC, hls.js)

---

## 5. Implementation Details

### 5.1 TranscodeToABR Implementation (Pseudocode)

```go
func (t *FFmpegTranscoder) TranscodeToABR(ctx context.Context, inputPath, outputDir string, variants []Variant) (*ABROutput, error) {
    // Validate inputs
    if err := t.validateInput(inputPath); err != nil {
        return nil, err
    }
    if err := t.validateOutputDir(outputDir); err != nil {
        return nil, err
    }
    if len(variants) == 0 {
        return nil, errors.New("at least one variant required")
    }

    var variantOutputs []VariantOutput

    // Process each variant sequentially
    for _, variant := range variants {
        // Create variant subdirectory
        variantDir := filepath.Join(outputDir, variant.Name)
        if err := os.MkdirAll(variantDir, 0755); err != nil {
            return nil, fmt.Errorf("create variant dir %s: %w", variant.Name, err)
        }

        // Transcode this variant
        output, err := t.transcodeVariant(ctx, inputPath, variantDir, variant)
        if err != nil {
            return nil, fmt.Errorf("transcode variant %s: %w", variant.Name, err)
        }

        variantOutputs = append(variantOutputs, *output)
    }

    // Generate master playlist
    masterPath := filepath.Join(outputDir, "master.m3u8")
    if err := t.generateMasterPlaylist(masterPath, variantOutputs); err != nil {
        return nil, fmt.Errorf("generate master playlist: %w", err)
    }

    return &ABROutput{
        MasterManifestPath: masterPath,
        Variants:          variantOutputs,
    }, nil
}
```

### 5.2 Master Playlist Generation

```go
func (t *FFmpegTranscoder) generateMasterPlaylist(path string, variants []VariantOutput) error {
    var sb strings.Builder
    sb.WriteString("#EXTM3U\n")
    sb.WriteString("#EXT-X-VERSION:3\n\n")

    for _, v := range variants {
        // Calculate approximate resolution (assuming 16:9 aspect ratio)
        width := v.Variant.Height * 16 / 9
        // Ensure width is even (codec requirement)
        if width%2 != 0 {
            width++
        }

        sb.WriteString(fmt.Sprintf(
            "#EXT-X-STREAM-INF:BANDWIDTH=%d,RESOLUTION=%dx%d\n",
            v.Variant.Bitrate, width, v.Variant.Height,
        ))
        sb.WriteString(fmt.Sprintf("%s/playlist.m3u8\n\n", v.Variant.Name))
    }

    return os.WriteFile(path, []byte(sb.String()), 0644)
}
```

### 5.3 Updated Upload Logic

```go
func (s *transcodeService) uploadABRFiles(ctx context.Context, outputKeyPrefix string, abrOutput *ABROutput) (string, error) {
    // Upload master manifest
    masterKey := outputKeyPrefix + "master.m3u8"
    if err := s.uploadFile(ctx, abrOutput.MasterManifestPath, masterKey, "application/vnd.apple.mpegurl"); err != nil {
        return "", fmt.Errorf("upload master manifest: %w", err)
    }

    // Upload each variant
    for _, variant := range abrOutput.Variants {
        variantPrefix := outputKeyPrefix + variant.Variant.Name + "/"

        // Upload variant playlist
        playlistKey := variantPrefix + "playlist.m3u8"
        if err := s.uploadFile(ctx, variant.ManifestPath, playlistKey, "application/vnd.apple.mpegurl"); err != nil {
            return "", fmt.Errorf("upload %s playlist: %w", variant.Variant.Name, err)
        }

        // Upload segments
        for _, segPath := range variant.SegmentPaths {
            segKey := variantPrefix + filepath.Base(segPath)
            if err := s.uploadFile(ctx, segPath, segKey, "video/mp2t"); err != nil {
                return "", fmt.Errorf("upload %s segment: %w", variant.Variant.Name, err)
            }
        }
    }

    return masterKey, nil
}
```

---

## 6. Breaking Changes & Migration

### API Changes
- **GET /v1/videos/{id}**: `hls_url` will point to `master.m3u8` instead of `playlist.m3u8`
- No breaking change for clients using standard HLS players (they expect master playlist)

### Storage Changes
- New directory structure under `hls/{videoID}/`
- Old videos (if any) will have `playlist.m3u8` at root level
- **Migration**: Not required for dev environment; for production, could add compatibility check

### Backward Compatibility
- Keep `TranscodeToHLS()` method functional (delegates to single 720p variant)
- This allows gradual migration if needed

---

## 7. Testing Strategy

### Unit Tests
1. `Variant` struct validation
2. `TranscodeToABR` with mocked FFmpeg execution
3. Master playlist generation format
4. Upload path construction

### Integration Tests
1. Full transcoding pipeline with test video
2. Verify all files uploaded to MinIO with correct structure
3. Verify master.m3u8 references correct relative paths

### Manual Testing
1. Upload video via API
2. Check MinIO bucket structure
3. Play back using VLC or hls.js demo page

---

## 8. Estimated File Changes

| File | Change Type | Description |
|------|-------------|-------------|
| `internal/transcoder/transcoder.go` | Modify | Add ABR types and interface |
| `internal/transcoder/ffmpeg.go` | Modify | Implement ABR transcoding |
| `internal/transcoder/ffmpeg_test.go` | Modify | Add ABR tests |
| `internal/usecase/transcode_service.go` | Modify | Use ABR transcoding |
| `internal/usecase/transcode_service_test.go` | Modify | Update tests |

---

## 9. Future Enhancements (Out of Scope)

These are intentionally deferred:
- **Parallel variant transcoding**: Run FFmpeg instances concurrently
- **Partial retry**: Retry only failed variants
- **Dynamic variant selection**: Based on source video resolution
- **Audio-only variant**: For bandwidth-constrained scenarios
- **Configurable variants via API**: Let users choose quality levels

---

## 10. Open Questions for Discussion

1. **Should we skip higher resolutions if source is lower?**
   - e.g., Don't generate 1080p if source is 720p
   - Recommendation: Yes, but defer to Phase 3 (requires video probing)

2. **Should variants be configurable per video?**
   - Recommendation: No, use system defaults for simplicity

3. **What if a variant transcoding fails partway through?**
   - Current design: Fail entire task, retry all
   - Future enhancement: Partial retry
