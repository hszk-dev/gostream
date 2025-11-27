import http from "k6/http";
import { check } from "k6";
import { getApiBase, getCdnBase } from "../config/environments.js";
import { TIMEOUTS } from "../config/constants.js";
import {
  recordNginxCacheStatus,
  recordLatencyBucket,
  getVideoLatency,
  hlsManifestLatency,
  hlsSegmentLatency,
} from "./metrics.js";

/**
 * API client for gostream load tests
 * Provides typed methods for API and CDN endpoints.
 */

const defaultParams = {
  timeout: TIMEOUTS.request,
};

/**
 * Get video metadata by ID.
 * Records latency metrics.
 *
 * NOTE: Cache hit/miss is measured via Prometheus (gostream_cache_operations_total),
 * not via k6 metrics.
 *
 * @param {string} videoId - Video UUID
 * @returns {Object} Response object with video data
 */
export function getVideo(videoId) {
  const url = `${getApiBase()}/v1/videos/${videoId}`;
  const res = http.get(url, defaultParams);
  const durationMs = res.timings.duration;

  // Record metrics
  getVideoLatency.add(durationMs);
  recordLatencyBucket(durationMs);

  // Validate response
  check(res, {
    "GET /v1/videos/{id} returns 200": (r) => r.status === 200,
    "response has video ID": (r) => {
      try {
        const body = JSON.parse(r.body);
        return body.id === videoId;
      } catch {
        return false;
      }
    },
  });

  return res;
}

/**
 * Create a new video and get presigned upload URL.
 *
 * @param {string} userId - User UUID
 * @param {string} title - Video title
 * @param {string} fileName - Original file name
 * @returns {Object} Response object with video data and upload URL
 */
export function createVideo(userId, title, fileName) {
  const url = `${getApiBase()}/v1/videos`;
  const payload = JSON.stringify({
    user_id: userId,
    title: title,
    file_name: fileName,
  });
  const params = {
    ...defaultParams,
    headers: { "Content-Type": "application/json" },
  };

  const res = http.post(url, payload, params);

  check(res, {
    "POST /v1/videos returns 201": (r) => r.status === 201,
    "response has upload_url": (r) => {
      try {
        const body = JSON.parse(r.body);
        return body.upload_url && body.upload_url.length > 0;
      } catch {
        return false;
      }
    },
  });

  return res;
}

/**
 * Trigger video processing.
 *
 * @param {string} videoId - Video UUID
 * @returns {Object} Response object
 */
export function triggerProcess(videoId) {
  const url = `${getApiBase()}/v1/videos/${videoId}/process`;
  const res = http.post(url, null, defaultParams);

  check(res, {
    "POST /v1/videos/{id}/process returns 202": (r) => r.status === 202,
  });

  return res;
}

/**
 * Get HLS manifest file from CDN.
 * Records Nginx cache and latency metrics.
 *
 * @param {string} videoId - Video UUID
 * @returns {Object} Response object with manifest content
 */
export function getHlsManifest(videoId) {
  const url = `${getCdnBase()}/hls/${videoId}/master.m3u8`;
  const res = http.get(url, defaultParams);
  const durationMs = res.timings.duration;

  // Record metrics
  hlsManifestLatency.add(durationMs);
  recordLatencyBucket(durationMs);

  // Check Nginx cache status
  const cacheStatus = res.headers["X-Cache-Status"];
  if (cacheStatus) {
    recordNginxCacheStatus(cacheStatus);
  }

  check(res, {
    "GET manifest returns 200": (r) => r.status === 200,
    "manifest has valid content": (r) =>
      r.body && r.body.includes("#EXTM3U"),
  });

  return res;
}

/**
 * Get HLS segment file from CDN.
 * Records Nginx cache and latency metrics.
 *
 * @param {string} videoId - Video UUID
 * @param {string} segmentName - Segment file name (e.g., "segment_000.ts")
 * @returns {Object} Response object with segment content
 */
export function getHlsSegment(videoId, segmentName) {
  const url = `${getCdnBase()}/hls/${videoId}/${segmentName}`;
  const res = http.get(url, defaultParams);
  const durationMs = res.timings.duration;

  // Record metrics
  hlsSegmentLatency.add(durationMs);
  recordLatencyBucket(durationMs);

  // Check Nginx cache status
  const cacheStatus = res.headers["X-Cache-Status"];
  if (cacheStatus) {
    recordNginxCacheStatus(cacheStatus);
  }

  check(res, {
    "GET segment returns 200": (r) => r.status === 200,
    "segment has content": (r) => r.body && r.body.length > 0,
  });

  return res;
}

/**
 * Health check endpoint.
 *
 * @returns {Object} Response object
 */
export function healthCheck() {
  const url = `${getApiBase()}/health`;
  const res = http.get(url, defaultParams);

  check(res, {
    "health check returns 200": (r) => r.status === 200,
  });

  return res;
}
