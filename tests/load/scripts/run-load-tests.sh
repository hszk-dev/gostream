#!/bin/bash
set -euo pipefail

#
# Load Test Runner
#
# Usage:
#   ./run-load-tests.sh [scenario] [options]
#
# Scenarios:
#   viral    - Scenario A: Viral Video (Singleflight test)
#   cdn      - Scenario B: CDN HLS Fetch (Coming soon)
#   mixed    - Scenario C: Mixed Workload (Coming soon)
#   baseline - Baseline: No Cache (Coming soon)
#
# Options:
#   --local    Run k6 locally (default)
#   --docker   Run k6 in Docker
#   --vus N    Override VU count
#   --duration Override duration
#

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
K6_DIR="$SCRIPT_DIR/../k6"
RESULTS_DIR="$SCRIPT_DIR/../results"
ENV_FILE="$PROJECT_ROOT/.loadtest.env"

# Defaults
SCENARIO="${1:-viral}"
RUN_MODE="local"
EXTRA_K6_ARGS=""

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# Parse arguments
shift || true
while [[ $# -gt 0 ]]; do
    case $1 in
        --local)
            RUN_MODE="local"
            shift
            ;;
        --docker)
            RUN_MODE="docker"
            shift
            ;;
        --vus)
            EXTRA_K6_ARGS="$EXTRA_K6_ARGS --vus $2"
            shift 2
            ;;
        --duration)
            EXTRA_K6_ARGS="$EXTRA_K6_ARGS --duration $2"
            shift 2
            ;;
        *)
            log_error "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Map scenario to script file
get_scenario_script() {
    case "$1" in
        viral|a)
            echo "scenarios/scenario-a-viral.js"
            ;;
        cdn|b)
            log_error "Scenario B (CDN) not yet implemented"
            exit 1
            ;;
        mixed|c)
            log_error "Scenario C (Mixed) not yet implemented"
            exit 1
            ;;
        baseline)
            log_error "Baseline scenario not yet implemented"
            exit 1
            ;;
        *)
            log_error "Unknown scenario: $1"
            log_info "Available: viral, cdn, mixed, baseline"
            exit 1
            ;;
    esac
}

# Load environment variables
load_env() {
    if [[ -f "$ENV_FILE" ]]; then
        log_info "Loading environment from $ENV_FILE"
        source "$ENV_FILE"
        export TEST_VIDEO_ID
    else
        log_warn "Environment file not found: $ENV_FILE"
        log_warn "Run setup-test-data.sh first, or set TEST_VIDEO_ID manually"
    fi
}

# Run k6 locally
run_local() {
    local script="$1"

    if ! command -v k6 &> /dev/null; then
        log_error "k6 not found. Install with: brew install k6"
        exit 1
    fi

    log_info "Running k6 locally..."
    k6 run \
        --out json="$RESULTS_DIR/$(basename "$script" .js)-$(date +%Y%m%d-%H%M%S).json" \
        $EXTRA_K6_ARGS \
        "$K6_DIR/$script"
}

# Run k6 in Docker
run_docker() {
    local script="$1"

    log_info "Running k6 in Docker..."
    docker compose --profile loadtest run --rm \
        -e TEST_VIDEO_ID="${TEST_VIDEO_ID:-}" \
        k6 run \
        --out influxdb=http://influxdb:8086/k6 \
        $EXTRA_K6_ARGS \
        "/tests/$script"
}

# Main
main() {
    log_info "=========================================="
    log_info "  GoStream Load Test Runner"
    log_info "=========================================="
    log_info "Scenario: $SCENARIO"
    log_info "Mode: $RUN_MODE"

    # Get scenario script
    local script
    script=$(get_scenario_script "$SCENARIO")
    log_info "Script: $script"

    # Load environment
    load_env

    # Ensure results directory exists
    mkdir -p "$RESULTS_DIR"

    # Run test
    log_info "Starting load test..."
    echo ""

    if [[ "$RUN_MODE" == "docker" ]]; then
        run_docker "$script"
    else
        run_local "$script"
    fi

    log_success "Load test completed!"
    log_info "Results saved to: $RESULTS_DIR/"
}

main
