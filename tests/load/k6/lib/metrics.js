import { Counter, Rate, Trend } from "k6/metrics";
import { LATENCY_BUCKETS } from "../config/constants.js";

/**
 * Custom metrics for load testing
 * Tracks Nginx CDN cache and latency distribution.
 *
 * NOTE: Redis cache and singleflight metrics are measured via Prometheus
 * (gostream_cache_operations_total, gostream_singleflight_requests_total)
 * for accurate measurement. Latency-based inference was removed due to inaccuracy.
 */

// Nginx Cache Metrics (based on X-Cache-Status header)
export const nginxCacheHits = new Counter("nginx_cache_hits");
export const nginxCacheMisses = new Counter("nginx_cache_misses");
export const nginxCacheHitRate = new Rate("nginx_cache_hit_rate");

// Latency Distribution
export const latencyUnder10ms = new Counter("latency_under_10ms");
export const latencyUnder50ms = new Counter("latency_under_50ms");
export const latencyUnder100ms = new Counter("latency_under_100ms");
export const latencyOver100ms = new Counter("latency_over_100ms");

// API-specific Trends
export const getVideoLatency = new Trend("get_video_latency", true);
export const hlsManifestLatency = new Trend("hls_manifest_latency", true);
export const hlsSegmentLatency = new Trend("hls_segment_latency", true);

/**
 * Record Nginx cache status from X-Cache-Status header.
 *
 * @param {string} cacheStatus - Value of X-Cache-Status header (HIT, MISS, EXPIRED, etc.)
 */
export function recordNginxCacheStatus(cacheStatus) {
  const isHit = cacheStatus === "HIT";
  if (isHit) {
    nginxCacheHits.add(1);
  } else {
    nginxCacheMisses.add(1);
  }
  nginxCacheHitRate.add(isHit);
}

/**
 * Record latency bucket for SLA tracking.
 *
 * @param {number} durationMs - Response time in milliseconds
 */
export function recordLatencyBucket(durationMs) {
  if (durationMs < LATENCY_BUCKETS.fast) {
    latencyUnder10ms.add(1);
  } else if (durationMs < LATENCY_BUCKETS.acceptable) {
    latencyUnder50ms.add(1);
  } else if (durationMs < LATENCY_BUCKETS.slow) {
    latencyUnder100ms.add(1);
  } else {
    latencyOver100ms.add(1);
  }
}
