import { Counter, Rate, Trend } from "k6/metrics";
import { CACHE_HIT_THRESHOLD_MS, LATENCY_BUCKETS } from "../config/constants.js";

/**
 * Custom metrics for load testing
 * Tracks cache performance, singleflight effectiveness, and latency distribution.
 */

// Cache Metrics
export const cacheHits = new Counter("cache_hits");
export const cacheMisses = new Counter("cache_misses");
export const cacheHitRate = new Rate("cache_hit_rate");

// Nginx Cache Metrics (based on X-Cache-Status header)
export const nginxCacheHits = new Counter("nginx_cache_hits");
export const nginxCacheMisses = new Counter("nginx_cache_misses");
export const nginxCacheHitRate = new Rate("nginx_cache_hit_rate");

// Singleflight Metrics (inferred from response time)
export const singleflightCoalesced = new Counter("singleflight_coalesced");
export const singleflightEffectiveness = new Rate("singleflight_effectiveness");

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
 * Record cache status based on response time inference.
 * Responses faster than CACHE_HIT_THRESHOLD_MS are considered cache hits.
 *
 * @param {number} durationMs - Response time in milliseconds
 */
export function recordCacheStatus(durationMs) {
  const isHit = durationMs < CACHE_HIT_THRESHOLD_MS;
  if (isHit) {
    cacheHits.add(1);
    singleflightCoalesced.add(1);
  } else {
    cacheMisses.add(1);
  }
  cacheHitRate.add(isHit);
  singleflightEffectiveness.add(isHit);
}

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
