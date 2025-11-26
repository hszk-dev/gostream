package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/hszk-dev/gostream/internal/domain/repository"
)

// ClientConfig holds configuration for the RabbitMQ client.
type ClientConfig struct {
	URL        string // AMQP connection URL (e.g., amqp://user:pass@host:port/vhost)
	QueueName  string // Queue name for transcode tasks
	Exchange   string // Exchange name (empty = default exchange)
	RoutingKey string // Routing key (typically same as queue name for default exchange)
	Prefetch   int    // Consumer prefetch count (QoS)
}

// DefaultClientConfig returns a ClientConfig with sensible defaults.
// Prefetch=1 ensures fair dispatch among multiple workers for CPU-intensive transcoding.
func DefaultClientConfig(url string) ClientConfig {
	return ClientConfig{
		URL:        url,
		QueueName:  "transcode_tasks",
		Exchange:   "", // Default exchange
		RoutingKey: "transcode_tasks",
		Prefetch:   1,
	}
}

// amqpConnection abstracts amqp.Connection for testability.
type amqpConnection interface {
	Channel() (*amqp.Channel, error)
	Close() error
	IsClosed() bool
}

// amqpChannel abstracts amqp.Channel for testability.
type amqpChannel interface {
	QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error)
	PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error
	Consume(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error)
	Qos(prefetchCount, prefetchSize int, global bool) error
	Close() error
}

// Client implements repository.MessageQueue using RabbitMQ.
type Client struct {
	conn    amqpConnection
	channel amqpChannel
	config  ClientConfig
}

// Compile-time verification that Client implements repository.MessageQueue.
var _ repository.MessageQueue = (*Client)(nil)

// NewClient creates a new RabbitMQ client.
// It establishes connection and declares the queue during initialization to fail fast.
func NewClient(ctx context.Context, cfg ClientConfig) (*Client, error) {
	conn, err := amqp.Dial(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	return newClientWithConnection(ctx, conn, cfg)
}

// newClientWithConnection creates a Client with a given amqpConnection.
// This is used for dependency injection in tests.
func newClientWithConnection(ctx context.Context, conn amqpConnection, cfg ClientConfig) (*Client, error) {
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close() // Best-effort cleanup; original error takes precedence
		return nil, fmt.Errorf("failed to open channel: %w", err)
	}

	if err := ch.Qos(cfg.Prefetch, 0, false); err != nil {
		_ = ch.Close()   // Best-effort cleanup
		_ = conn.Close() // Best-effort cleanup
		return nil, fmt.Errorf("failed to set QoS: %w", err)
	}

	// Declare queue (idempotent operation)
	// durable=true ensures queue survives broker restart
	_, err = ch.QueueDeclare(
		cfg.QueueName,
		true,  // durable
		false, // autoDelete
		false, // exclusive
		false, // noWait
		nil,   // arguments
	)
	if err != nil {
		_ = ch.Close()   // Best-effort cleanup
		_ = conn.Close() // Best-effort cleanup
		return nil, fmt.Errorf("failed to declare queue: %w", err)
	}

	return &Client{
		conn:    conn,
		channel: ch,
		config:  cfg,
	}, nil
}

// PublishTranscodeTask sends a transcoding task to the queue.
// Messages are persistent to survive broker restarts.
func (c *Client) PublishTranscodeTask(ctx context.Context, task repository.TranscodeTask) error {
	body, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("failed to marshal task: %w", err)
	}

	err = c.channel.PublishWithContext(
		ctx,
		c.config.Exchange,
		c.config.RoutingKey,
		false, // mandatory
		false, // immediate
		amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "application/json",
			Body:         body,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to publish task: %w", err)
	}

	return nil
}

// ConsumeTranscodeTasks starts consuming transcoding tasks from the queue.
// The handler function is called for each received task.
// Returns when context is cancelled or channel is closed.
//
// Ack/Nack strategy:
//   - Successful processing: Ack
//   - JSON unmarshal failure: Nack without requeue (malformed message)
//   - Handler failure: Increment RetryCount, republish as new message, Ack original
//
// Note: We don't use Nack(requeue=true) for retries because it would put the
// same message back without incrementing RetryCount, causing an infinite loop.
func (c *Client) ConsumeTranscodeTasks(ctx context.Context, handler func(task repository.TranscodeTask) error) error {
	msgs, err := c.channel.Consume(
		c.config.QueueName,
		"",    // consumer tag (auto-generated)
		false, // autoAck - manual ack for reliability
		false, // exclusive
		false, // noLocal
		false, // noWait
		nil,   // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to register consumer: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-msgs:
			if !ok {
				return fmt.Errorf("message channel closed unexpectedly")
			}

			var task repository.TranscodeTask
			if err := json.Unmarshal(msg.Body, &task); err != nil {
				// Malformed message - don't requeue
				_ = msg.Nack(false, false)
				continue
			}

			if err := handler(task); err != nil {
				// Processing failed - increment retry count and republish
				task.RetryCount++
				if pubErr := c.PublishTranscodeTask(ctx, task); pubErr != nil {
					// Republish failed - discard message to prevent infinite loop
					// The video will remain in PROCESSING state for manual investigation
					slog.Error("failed to republish task for retry",
						"video_id", task.VideoID,
						"retry_count", task.RetryCount,
						"error", pubErr,
					)
					_ = msg.Nack(false, false)
				} else {
					// Republish succeeded - ack original message
					_ = msg.Ack(false)
				}
				continue
			}

			_ = msg.Ack(false)
		}
	}
}

// Close gracefully closes the RabbitMQ connection and channel.
func (c *Client) Close() error {
	var errs []error

	if c.channel != nil {
		if err := c.channel.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close channel: %w", err))
		}
	}

	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close connection: %w", err))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
