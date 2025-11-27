import { sleep } from "k6";
import { getVideo, healthCheck } from "../lib/api-client.js";
import { getTestVideoId, logWithTimestamp } from "../lib/utils.js";
import { THRESHOLDS } from "../config/constants.js";

/**
 * Scenario A: Viral Video (Singleflight + Redis Cache Verification)
 *
 * Purpose:
 * - 100 VUs simultaneously access the SAME video ID
 * - Verify Singleflight coalesces concurrent DB queries into 1
 * - Verify Redis cache hit ratio > 90% after warmup
 *
 * Expected Results:
 * - p95 latency < 50ms
 * - Cache hit ratio > 90%
 * - DB queries should be ~12/min (TTL 5min) instead of ~6000/min (100 VU × 60 RPS)
 */

export const options = {
  scenarios: {
    viral_video: {
      executor: "ramping-vus",
      startVUs: 0,
      stages: [
        // Ramp up to 100 VUs over 10 seconds
        { duration: "10s", target: 100 },
        // Hold at 100 VUs for 60 seconds (main test phase)
        { duration: "60s", target: 100 },
        // Ramp down over 10 seconds
        { duration: "10s", target: 0 },
      ],
      gracefulRampDown: "5s",
    },
  },
  thresholds: {
    // Response time thresholds
    http_req_duration: [`p(95)<${THRESHOLDS.viral.http_req_duration_p95}`],
    get_video_latency: [`p(95)<${THRESHOLDS.viral.http_req_duration_p95}`],

    // Cache effectiveness
    cache_hit_rate: [`rate>${THRESHOLDS.viral.cache_hit_ratio}`],
    singleflight_effectiveness: [`rate>${THRESHOLDS.viral.cache_hit_ratio}`],

    // Error rate
    http_req_failed: [`rate<${THRESHOLDS.viral.error_rate}`],

    // Check assertions
    checks: ["rate>0.99"],
  },
  // Summary output
  summaryTrendStats: ["avg", "min", "med", "max", "p(90)", "p(95)", "p(99)"],
};

// Get the test video ID from environment
const VIDEO_ID = getTestVideoId();

/**
 * Setup function - runs once before the test starts.
 * Validates that the API is healthy and test video exists.
 */
export function setup() {
  logWithTimestamp(`Starting Scenario A: Viral Video Test`);
  logWithTimestamp(`Target Video ID: ${VIDEO_ID}`);

  // Health check
  const healthRes = healthCheck();
  if (healthRes.status !== 200) {
    throw new Error(`API health check failed: ${healthRes.status}`);
  }
  logWithTimestamp("API health check passed");

  // Verify test video exists
  const videoRes = getVideo(VIDEO_ID);
  if (videoRes.status !== 200) {
    throw new Error(
      `Test video not found: ${VIDEO_ID}. Run setup-test-data.sh first.`
    );
  }

  const video = JSON.parse(videoRes.body);
  logWithTimestamp(`Test video found: "${video.title}" (status: ${video.status})`);

  return { videoId: VIDEO_ID };
}

/**
 * Main test function - runs for each VU iteration.
 * All VUs access the same video ID to test Singleflight effectiveness.
 */
export default function (data) {
  // All VUs request the same video
  getVideo(data.videoId);

  // Small sleep to prevent overwhelming the server
  // With 100 VUs and ~0.1s sleep, we get ~1000 RPS
  sleep(0.1);
}

/**
 * Teardown function - runs once after the test completes.
 */
export function teardown(data) {
  logWithTimestamp("Scenario A: Viral Video Test completed");
  logWithTimestamp(`Video ID tested: ${data.videoId}`);
  logWithTimestamp("Check pg_stat_statements for DB query counts");
}

/**
 * Custom summary handler for detailed results.
 */
export function handleSummary(data) {
  const summary = {
    scenario: "Scenario A: Viral Video",
    videoId: VIDEO_ID,
    timestamp: new Date().toISOString(),
    metrics: {
      totalRequests: data.metrics.http_reqs?.values?.count || 0,
      avgDuration: data.metrics.http_req_duration?.values?.avg || 0,
      p95Duration: data.metrics.http_req_duration?.values["p(95)"] || 0,
      p99Duration: data.metrics.http_req_duration?.values["p(99)"] || 0,
      cacheHitRate: data.metrics.cache_hit_rate?.values?.rate || 0,
      errorRate: data.metrics.http_req_failed?.values?.rate || 0,
    },
    thresholds: {
      passed: Object.entries(data.metrics)
        .filter(([_, v]) => v.thresholds)
        .every(([_, v]) => Object.values(v.thresholds).every((t) => t.ok)),
    },
  };

  console.log("\n========== SCENARIO A RESULTS ==========");
  console.log(`Total Requests: ${summary.metrics.totalRequests}`);
  console.log(`Avg Duration: ${summary.metrics.avgDuration.toFixed(2)}ms`);
  console.log(`p95 Duration: ${summary.metrics.p95Duration.toFixed(2)}ms`);
  console.log(`p99 Duration: ${summary.metrics.p99Duration.toFixed(2)}ms`);
  console.log(`Cache Hit Rate: ${(summary.metrics.cacheHitRate * 100).toFixed(1)}%`);
  console.log(`Error Rate: ${(summary.metrics.errorRate * 100).toFixed(2)}%`);
  console.log(`All Thresholds Passed: ${summary.thresholds.passed}`);
  console.log("==========================================\n");

  return {
    "/results/scenario-a-viral.json": JSON.stringify(summary, null, 2),
    stdout: textSummary(data, { indent: " ", enableColors: true }),
  };
}

// Import text summary for stdout
import { textSummary } from "https://jslib.k6.io/k6-summary/0.0.2/index.js";
