/**
 * Constants for k6 load tests
 * Defines thresholds, timeouts, and other shared configuration values.
 */

// Thresholds for different scenarios
export const THRESHOLDS = {
  // Scenario A: Viral Video (Singleflight + Redis)
  viral: {
    http_req_duration_p95: 50, // ms
    cache_hit_ratio: 0.9, // 90%
    error_rate: 0.01, // 1%
  },

  // Scenario B: CDN HLS Fetch
  cdn: {
    manifest_p95: 20, // ms
    segment_p95: 50, // ms
    cache_hit_ratio: 0.9, // 90%
  },

  // Scenario C: Mixed Workload
  mixed: {
    read_p95: 50, // ms
    error_rate: 0.02, // 2%
  },

  // Baseline: No Cache
  baseline: {
    http_req_duration_p95: 500, // ms
  },
};

// Timeouts
export const TIMEOUTS = {
  request: "10s",
  connect: "5s",
};

// Latency buckets for SLA tracking (in milliseconds)
export const LATENCY_BUCKETS = {
  fast: 10,
  acceptable: 50,
  slow: 100,
};

// Cache inference threshold (responses faster than this are likely cache hits)
export const CACHE_HIT_THRESHOLD_MS = 5;
