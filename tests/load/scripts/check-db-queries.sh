#!/bin/bash
set -euo pipefail

#
# Check Database Queries (pg_stat_statements)
#
# Verifies Singleflight effectiveness by checking DB query counts.
#
# Usage:
#   ./check-db-queries.sh [--reset]
#
# Options:
#   --reset    Reset pg_stat_statements before running load test
#

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

POSTGRES_CONTAINER="gostream-postgres"
POSTGRES_USER="${POSTGRES_USER:-gostream}"
POSTGRES_DB="${POSTGRES_DB:-gostream}"

# Run SQL command
run_sql() {
    docker exec "$POSTGRES_CONTAINER" psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$1"
}

# Check if pg_stat_statements is available
check_extension() {
    log_info "Checking pg_stat_statements extension..."

    local result
    result=$(run_sql "SELECT COUNT(*) FROM pg_extension WHERE extname = 'pg_stat_statements';" 2>/dev/null | grep -E '^\s*[0-9]+' | tr -d ' ')

    if [[ "$result" != "1" ]]; then
        log_error "pg_stat_statements extension is not installed"
        log_info "Add to docker-compose.yml postgres service:"
        log_info '  command: ["postgres", "-c", "shared_preload_libraries=pg_stat_statements"]'
        exit 1
    fi
    log_success "pg_stat_statements is available"
}

# Reset statistics
reset_stats() {
    log_info "Resetting pg_stat_statements..."
    run_sql "SELECT pg_stat_statements_reset();" > /dev/null
    log_success "Statistics reset"
}

# Show video-related queries
show_video_queries() {
    log_info "Video-related query statistics:"
    echo ""

    run_sql "
SELECT
    substring(query, 1, 80) as query_preview,
    calls,
    round(total_exec_time::numeric, 2) as total_time_ms,
    round((total_exec_time / calls)::numeric, 2) as avg_time_ms,
    rows
FROM pg_stat_statements
WHERE query ILIKE '%videos%'
  AND query NOT ILIKE '%pg_stat%'
ORDER BY calls DESC
LIMIT 10;
"
}

# Analyze Singleflight effectiveness
analyze_singleflight() {
    log_info "=========================================="
    log_info "  Singleflight Effectiveness Analysis"
    log_info "=========================================="

    local select_calls
    select_calls=$(run_sql "
SELECT COALESCE(SUM(calls), 0)
FROM pg_stat_statements
WHERE query ILIKE '%SELECT%videos%'
  AND query ILIKE '%WHERE%id%'
  AND query NOT ILIKE '%pg_stat%';
" 2>/dev/null | grep -E '^\s*[0-9]+' | tr -d ' ')

    echo ""
    echo -e "${CYAN}SELECT queries on videos table: ${NC}${select_calls:-0}"
    echo ""

    if [[ "${select_calls:-0}" -gt 0 ]]; then
        log_info "Expected behavior with Singleflight:"
        log_info "  - 100 VU × 60s × ~10 RPS = ~60,000 requests"
        log_info "  - With Singleflight + 5min TTL: ~12 DB queries/min"
        log_info "  - Total expected: ~12-20 DB queries"
        echo ""

        if [[ "$select_calls" -lt 100 ]]; then
            log_success "Singleflight is working effectively!"
            log_info "DB queries are significantly reduced"
        elif [[ "$select_calls" -lt 1000 ]]; then
            log_warn "Singleflight may be partially effective"
            log_info "Check cache TTL and concurrent request patterns"
        else
            log_error "Singleflight may not be working as expected"
            log_info "Expected ~12-20 queries, got $select_calls"
        fi
    fi
}

# Main
main() {
    local reset_mode=false

    while [[ $# -gt 0 ]]; do
        case $1 in
            --reset)
                reset_mode=true
                shift
                ;;
            *)
                log_error "Unknown option: $1"
                exit 1
                ;;
        esac
    done

    log_info "=========================================="
    log_info "  Database Query Analysis"
    log_info "=========================================="

    check_extension

    if [[ "$reset_mode" == "true" ]]; then
        reset_stats
        log_info ""
        log_info "Statistics reset. Run your load test, then run this script again without --reset"
        exit 0
    fi

    show_video_queries
    analyze_singleflight
}

main "$@"
