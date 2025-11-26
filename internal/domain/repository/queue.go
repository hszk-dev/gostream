package repository

import (
	"context"

	"github.com/google/uuid"
)

// TranscodeTask represents a video transcoding job message.
type TranscodeTask struct {
	VideoID     uuid.UUID `json:"video_id"`
	OriginalKey string    `json:"original_key"`
	OutputKey   string    `json:"output_key"`
	RetryCount  int       `json:"retry_count"`
}

// MessageQueue defines the interface for message queue operations.
// Implementations should be provided by the infrastructure layer (e.g., RabbitMQ).
type MessageQueue interface {
	// PublishTranscodeTask sends a transcoding task to the queue.
	// Used by the API server to trigger async video processing.
	PublishTranscodeTask(ctx context.Context, task TranscodeTask) error

	// ConsumeTranscodeTasks starts consuming transcoding tasks from the queue.
	// The handler function is called for each received task.
	// Returns a channel that can be used to stop consumption.
	// Used by the worker service.
	ConsumeTranscodeTasks(ctx context.Context, handler func(task TranscodeTask) error) error

	// Close gracefully closes the connection to the message queue.
	Close() error
}
