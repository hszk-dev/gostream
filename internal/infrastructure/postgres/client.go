package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ClientConfig holds configuration for the PostgreSQL client.
type ClientConfig struct {
	DSN             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

// DefaultClientConfig returns a ClientConfig with sensible defaults.
func DefaultClientConfig(dsn string) ClientConfig {
	return ClientConfig{
		DSN:             dsn,
		MaxConns:        25,
		MinConns:        5,
		MaxConnLifetime: time.Hour,
		MaxConnIdleTime: 30 * time.Minute,
	}
}

// Client wraps a PostgreSQL connection pool.
type Client struct {
	pool *pgxpool.Pool
}

// NewClient creates a new PostgreSQL client with connection pooling.
func NewClient(ctx context.Context, cfg ClientConfig) (*Client, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("failed to parse DSN: %w", err)
	}

	poolConfig.MaxConns = cfg.MaxConns
	poolConfig.MinConns = cfg.MinConns
	poolConfig.MaxConnLifetime = cfg.MaxConnLifetime
	poolConfig.MaxConnIdleTime = cfg.MaxConnIdleTime

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &Client{pool: pool}, nil
}

// Pool returns the underlying connection pool.
// Use this for creating repository instances.
func (c *Client) Pool() *pgxpool.Pool {
	return c.pool
}

// Ping verifies the database connection is alive.
func (c *Client) Ping(ctx context.Context) error {
	return c.pool.Ping(ctx)
}

// Close closes all connections in the pool.
func (c *Client) Close() {
	c.pool.Close()
}

// Stats returns connection pool statistics.
type Stats struct {
	AcquireCount         int64
	AcquiredConns        int32
	IdleConns            int32
	TotalConns           int32
	MaxConns             int32
	EmptyAcquireCount    int64
	CanceledAcquireCount int64
}

// Stats returns current connection pool statistics.
func (c *Client) Stats() Stats {
	s := c.pool.Stat()
	return Stats{
		AcquireCount:         s.AcquireCount(),
		AcquiredConns:        s.AcquiredConns(),
		IdleConns:            s.IdleConns(),
		TotalConns:           s.TotalConns(),
		MaxConns:             s.MaxConns(),
		EmptyAcquireCount:    s.EmptyAcquireCount(),
		CanceledAcquireCount: s.CanceledAcquireCount(),
	}
}
