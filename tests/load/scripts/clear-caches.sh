#!/bin/bash
set -euo pipefail

#
# Clear All Caches
#
# Clears Redis cache and Nginx cache for baseline testing.
#
# Usage:
#   ./clear-caches.sh [--redis] [--nginx] [--all]
#

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

CLEAR_REDIS=false
CLEAR_NGINX=false

# Parse arguments
if [[ $# -eq 0 ]]; then
    # Default: clear all
    CLEAR_REDIS=true
    CLEAR_NGINX=true
else
    while [[ $# -gt 0 ]]; do
        case $1 in
            --redis)
                CLEAR_REDIS=true
                shift
                ;;
            --nginx)
                CLEAR_NGINX=true
                shift
                ;;
            --all)
                CLEAR_REDIS=true
                CLEAR_NGINX=true
                shift
                ;;
            *)
                log_error "Unknown option: $1"
                log_info "Usage: $0 [--redis] [--nginx] [--all]"
                exit 1
                ;;
        esac
    done
fi

# Clear Redis cache
clear_redis() {
    log_info "Clearing Redis cache..."

    if docker exec gostream-redis redis-cli FLUSHALL 2>/dev/null; then
        log_success "Redis cache cleared"
    else
        log_warn "Failed to clear Redis cache (is Redis running?)"
    fi
}

# Clear Nginx cache
clear_nginx() {
    log_info "Clearing Nginx cache..."

    # Clear cache directory inside container
    if docker exec gostream-nginx rm -rf /var/cache/nginx/hls/* 2>/dev/null; then
        log_success "Nginx cache directory cleared"
    else
        log_warn "Nginx cache directory might already be empty"
    fi

    # Reload Nginx to ensure clean state
    if docker exec gostream-nginx nginx -s reload 2>/dev/null; then
        log_success "Nginx reloaded"
    else
        log_warn "Failed to reload Nginx"
    fi
}

# Main
main() {
    log_info "=========================================="
    log_info "  Clear Caches for Load Testing"
    log_info "=========================================="

    if [[ "$CLEAR_REDIS" == "true" ]]; then
        clear_redis
    fi

    if [[ "$CLEAR_NGINX" == "true" ]]; then
        clear_nginx
    fi

    log_success "Cache clearing complete!"
}

main
