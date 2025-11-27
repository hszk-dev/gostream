import { sleep } from "k6";

/**
 * Utility functions for k6 load tests
 */

/**
 * Generate a random UUID v4.
 * Used for creating unique user IDs and video IDs.
 *
 * @returns {string} Random UUID
 */
export function generateUUID() {
  return "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx".replace(/[xy]/g, function (c) {
    const r = (Math.random() * 16) | 0;
    const v = c === "x" ? r : (r & 0x3) | 0x8;
    return v.toString(16);
  });
}

/**
 * Sleep for a random duration within a range.
 * Useful for simulating realistic user behavior.
 *
 * @param {number} minSeconds - Minimum sleep duration
 * @param {number} maxSeconds - Maximum sleep duration
 */
export function randomSleep(minSeconds, maxSeconds) {
  const duration = minSeconds + Math.random() * (maxSeconds - minSeconds);
  sleep(duration);
}

/**
 * Parse JSON response body safely.
 *
 * @param {Object} response - k6 response object
 * @returns {Object|null} Parsed JSON or null on error
 */
export function parseJsonResponse(response) {
  try {
    return JSON.parse(response.body);
  } catch (e) {
    console.error(`Failed to parse JSON response: ${e.message}`);
    return null;
  }
}

/**
 * Get test video ID from environment variable.
 * Falls back to a default UUID if not set.
 *
 * @returns {string} Video ID to use for testing
 */
export function getTestVideoId() {
  return __ENV.TEST_VIDEO_ID || "00000000-0000-0000-0000-000000000000";
}

/**
 * Format duration in milliseconds to human-readable string.
 *
 * @param {number} ms - Duration in milliseconds
 * @returns {string} Formatted duration
 */
export function formatDuration(ms) {
  if (ms < 1) {
    return `${(ms * 1000).toFixed(2)}µs`;
  } else if (ms < 1000) {
    return `${ms.toFixed(2)}ms`;
  } else {
    return `${(ms / 1000).toFixed(2)}s`;
  }
}

/**
 * Log a message with timestamp prefix.
 *
 * @param {string} message - Message to log
 */
export function logWithTimestamp(message) {
  const now = new Date().toISOString();
  console.log(`[${now}] ${message}`);
}

/**
 * Calculate percentile from an array of numbers.
 *
 * @param {number[]} arr - Array of numbers
 * @param {number} p - Percentile (0-100)
 * @returns {number} Percentile value
 */
export function percentile(arr, p) {
  if (arr.length === 0) return 0;
  const sorted = [...arr].sort((a, b) => a - b);
  const index = Math.ceil((p / 100) * sorted.length) - 1;
  return sorted[Math.max(0, index)];
}
