/**
 * Environment configuration for k6 load tests
 * Supports local development and Docker execution.
 */

const environments = {
  // Local development (running k6 directly on host)
  local: {
    apiBase: "http://localhost:8080",
    cdnBase: "http://localhost:8081",
    name: "local",
  },

  // Docker execution (running k6 inside docker-compose network)
  docker: {
    apiBase: "http://api:8080",
    cdnBase: "http://nginx:80",
    name: "docker",
  },
};

/**
 * Get environment configuration based on TEST_ENV variable.
 * @returns {Object} Environment configuration
 */
export function getEnvironment() {
  const envName = __ENV.TEST_ENV || "local";
  const env = environments[envName];

  if (!env) {
    console.error(`Unknown environment: ${envName}. Using 'local' as fallback.`);
    return environments.local;
  }

  return env;
}

/**
 * Get API base URL, allowing override via API_BASE env variable.
 * @returns {string} API base URL
 */
export function getApiBase() {
  return __ENV.API_BASE || getEnvironment().apiBase;
}

/**
 * Get CDN base URL, allowing override via CDN_BASE env variable.
 * @returns {string} CDN base URL
 */
export function getCdnBase() {
  return __ENV.CDN_BASE || getEnvironment().cdnBase;
}
