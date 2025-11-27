// Package metrics provides Prometheus metrics for observability.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const namespace = "gostream"

var (
	// CacheOperationsTotal tracks cache operations (get, set, delete).
	// Labels:
	//   - operation: get, set, delete
	//   - status: hit, miss, success, error
	//   - cache_type: redis
	CacheOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "cache_operations_total",
			Help:      "Total number of cache operations",
		},
		[]string{"operation", "status", "cache_type"},
	)

	// DBQueriesTotal tracks database queries.
	// Labels:
	//   - query_type: select, insert, update, delete
	//   - table: videos
	DBQueriesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "db_queries_total",
			Help:      "Total number of database queries",
		},
		[]string{"query_type", "table"},
	)

	// SingleflightRequestsTotal tracks singleflight behavior.
	// Labels:
	//   - result: initiated (new execution), shared (reused result)
	SingleflightRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "singleflight_requests_total",
			Help:      "Total number of singleflight requests",
		},
		[]string{"result"},
	)
)

// Cache operation status constants.
const (
	CacheStatusHit     = "hit"
	CacheStatusMiss    = "miss"
	CacheStatusSuccess = "success"
	CacheStatusError   = "error"
)

// Cache operation type constants.
const (
	CacheOpGet    = "get"
	CacheOpSet    = "set"
	CacheOpDelete = "delete"
)

// Cache type constants.
const (
	CacheTypeRedis = "redis"
)

// DB query type constants.
const (
	DBQuerySelect = "select"
	DBQueryInsert = "insert"
	DBQueryUpdate = "update"
)

// Table name constants.
const (
	TableVideos = "videos"
)

// Singleflight result constants.
const (
	SingleflightInitiated = "initiated"
	SingleflightShared    = "shared"
)
