/**
 * Constants for k6 load tests
 * Defines thresholds, timeouts, and other shared configuration values.
 */

// Thresholds for different scenarios
// NOTE: Cache hit ratio is measured via Prometheus (gostream_cache_operations_total),
// not via k6 metrics. Only latency and error rate thresholds are enforced here.
export const THRESHOLDS = {
  // Scenario A: Viral Video (Singleflight + Redis)
  viral: {
    http_req_duration_p95: 50, // ms
    error_rate: 0.01, // 1%
  },

  // Scenario B: CDN HLS Fetch
  cdn: {
    manifest_p95: 20, // ms
    segment_p95: 50, // ms
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
