#!/bin/bash
set -euo pipefail

#
# Setup Test Data for Load Testing
#
# This script:
# 1. Creates video metadata via API
# 2. Uploads a sample video file to MinIO using presigned URL
# 3. Triggers transcoding process
# 4. Waits for video to reach READY status
# 5. Saves TEST_VIDEO_ID to .loadtest.env
#

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
ENV_FILE="$PROJECT_ROOT/.loadtest.env"

# Configuration
API_BASE="${API_BASE:-http://localhost:8080}"
USER_ID="${USER_ID:-00000000-0000-0000-0000-000000000001}"
VIDEO_TITLE="Load Test Video"
POLL_INTERVAL=5
MAX_POLL_ATTEMPTS=60  # 5 minutes max wait

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() { echo -e "${BLUE}[INFO]${NC} $1" >&2; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1" >&2; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1" >&2; }
log_error() { echo -e "${RED}[ERROR]${NC} $1" >&2; }

# Check dependencies
check_dependencies() {
    local deps=("curl" "jq")
    for dep in "${deps[@]}"; do
        if ! command -v "$dep" &> /dev/null; then
            log_error "Required dependency not found: $dep"
            exit 1
        fi
    done
}

# Create sample video using FFmpeg (if available)
create_sample_video() {
    local output_file="$1"

    if command -v ffmpeg &> /dev/null; then
        log_info "Creating sample video with FFmpeg..."
        ffmpeg -y -f lavfi -i testsrc=duration=10:size=640x360:rate=30 \
               -f lavfi -i sine=frequency=440:duration=10 \
               -c:v libx264 -preset ultrafast -crf 28 \
               -c:a aac -b:a 64k \
               "$output_file" 2>/dev/null
        log_success "Sample video created: $output_file"
    else
        log_warn "FFmpeg not found. Creating minimal test file..."
        # Create a minimal valid file (will fail transcoding but works for API testing)
        echo "test" > "$output_file"
    fi
}

# Check API health
check_api_health() {
    log_info "Checking API health..."
    local response
    response=$(curl -s -w "\n%{http_code}" "$API_BASE/health" 2>/dev/null || echo -e "\n000")
    local http_code=$(echo "$response" | tail -n1)

    if [[ "$http_code" != "200" ]]; then
        log_error "API is not healthy. HTTP $http_code"
        log_error "Make sure services are running: make up"
        exit 1
    fi
    log_success "API is healthy"
}

# Create video metadata
create_video() {
    local file_name="$1"
    log_info "Creating video metadata..."

    local response
    response=$(curl -s -X POST "$API_BASE/v1/videos" \
        -H "Content-Type: application/json" \
        -d "{
            \"user_id\": \"$USER_ID\",
            \"title\": \"$VIDEO_TITLE\",
            \"file_name\": \"$file_name\"
        }")

    local video_id=$(echo "$response" | jq -r '.id // empty')
    local upload_url=$(echo "$response" | jq -r '.upload_url // empty')

    if [[ -z "$video_id" || -z "$upload_url" ]]; then
        log_error "Failed to create video: $response"
        exit 1
    fi

    log_success "Video created: $video_id"
    echo "$video_id|$upload_url"
}

# Upload video file
upload_video() {
    local upload_url="$1"
    local file_path="$2"

    log_info "Uploading video file..."

    # Convert host.docker.internal to localhost for local access
    # but keep Host header as host.docker.internal for presigned URL signature validation
    local original_host=""
    local local_url="$upload_url"

    if [[ "$upload_url" == *"host.docker.internal"* ]]; then
        original_host="host.docker.internal:9000"
        local_url=$(echo "$upload_url" | sed 's/host\.docker\.internal/localhost/g')
    fi

    local http_code
    if [[ -n "$original_host" ]]; then
        http_code=$(curl -s -o /dev/null -w "%{http_code}" -X PUT \
            -H "Content-Type: video/mp4" \
            -H "Host: $original_host" \
            --data-binary "@$file_path" \
            "$local_url")
    else
        http_code=$(curl -s -o /dev/null -w "%{http_code}" -X PUT \
            -H "Content-Type: video/mp4" \
            --data-binary "@$file_path" \
            "$upload_url")
    fi

    if [[ "$http_code" != "200" ]]; then
        log_error "Failed to upload video. HTTP $http_code"
        exit 1
    fi
    log_success "Video uploaded successfully"
}

# Trigger transcoding
trigger_process() {
    local video_id="$1"
    log_info "Triggering transcoding process..."

    local http_code
    http_code=$(curl -s -o /dev/null -w "%{http_code}" -X POST \
        "$API_BASE/v1/videos/$video_id/process")

    if [[ "$http_code" != "202" ]]; then
        log_error "Failed to trigger process. HTTP $http_code"
        exit 1
    fi
    log_success "Transcoding triggered"
}

# Poll for video status
wait_for_ready() {
    local video_id="$1"
    log_info "Waiting for video to be ready (max $((MAX_POLL_ATTEMPTS * POLL_INTERVAL))s)..."

    for ((i=1; i<=MAX_POLL_ATTEMPTS; i++)); do
        local response
        response=$(curl -s "$API_BASE/v1/videos/$video_id")
        local status=$(echo "$response" | jq -r '.status // empty')

        case "$status" in
            "READY")
                log_success "Video is READY!"
                return 0
                ;;
            "FAILED")
                log_error "Video processing FAILED"
                return 1
                ;;
            "PROCESSING")
                echo -ne "\r${YELLOW}[INFO]${NC} Status: PROCESSING (attempt $i/$MAX_POLL_ATTEMPTS)..."
                ;;
            *)
                echo -ne "\r${YELLOW}[INFO]${NC} Status: $status (attempt $i/$MAX_POLL_ATTEMPTS)..."
                ;;
        esac
        sleep "$POLL_INTERVAL"
    done

    echo ""
    log_error "Timeout waiting for video to be ready"
    return 1
}

# Save environment file
save_env_file() {
    local video_id="$1"

    cat > "$ENV_FILE" << EOF
# Load test environment variables
# Generated by setup-test-data.sh on $(date -u +"%Y-%m-%dT%H:%M:%SZ")
TEST_VIDEO_ID=$video_id
EOF

    log_success "Environment saved to $ENV_FILE"
}

# Main
main() {
    log_info "=========================================="
    log_info "  GoStream Load Test Data Setup"
    log_info "=========================================="

    check_dependencies
    check_api_health

    # Create temporary directory for sample video
    local tmp_dir=$(mktemp -d)
    local sample_video="$tmp_dir/sample.mp4"
    trap "rm -rf $tmp_dir" EXIT

    # Create sample video
    create_sample_video "$sample_video"

    # Create video and get upload URL
    local result
    result=$(create_video "sample.mp4")
    local video_id=$(echo "$result" | cut -d'|' -f1)
    local upload_url=$(echo "$result" | cut -d'|' -f2)

    # Upload video
    upload_video "$upload_url" "$sample_video"

    # Trigger processing
    trigger_process "$video_id"

    # Wait for completion
    if wait_for_ready "$video_id"; then
        save_env_file "$video_id"

        log_info "=========================================="
        log_success "Setup complete!"
        log_info "  Video ID: $video_id"
        log_info "  Env file: $ENV_FILE"
        log_info ""
        log_info "Run load tests with:"
        log_info "  source $ENV_FILE && make loadtest-viral"
        log_info "=========================================="
    else
        log_error "Setup failed. Check worker logs for details."
        exit 1
    fi
}

main "$@"
