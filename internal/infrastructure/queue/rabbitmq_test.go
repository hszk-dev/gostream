package queue

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/hszk-dev/gostream/internal/domain/repository"
)

// mockConnection implements amqpConnection interface for testing.
type mockConnection struct {
	channelFunc  func() (*amqp.Channel, error)
	closeFunc    func() error
	isClosedFunc func() bool
}

func (m *mockConnection) Channel() (*amqp.Channel, error) {
	if m.channelFunc != nil {
		return m.channelFunc()
	}
	return nil, nil
}

func (m *mockConnection) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

func (m *mockConnection) IsClosed() bool {
	if m.isClosedFunc != nil {
		return m.isClosedFunc()
	}
	return false
}

// mockChannel implements amqpChannel interface for testing.
type mockChannel struct {
	queueDeclareFunc       func(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error)
	publishWithContextFunc func(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error
	consumeFunc            func(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error)
	qosFunc                func(prefetchCount, prefetchSize int, global bool) error
	closeFunc              func() error
}

func (m *mockChannel) QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error) {
	if m.queueDeclareFunc != nil {
		return m.queueDeclareFunc(name, durable, autoDelete, exclusive, noWait, args)
	}
	return amqp.Queue{Name: name}, nil
}

func (m *mockChannel) PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error {
	if m.publishWithContextFunc != nil {
		return m.publishWithContextFunc(ctx, exchange, key, mandatory, immediate, msg)
	}
	return nil
}

func (m *mockChannel) Consume(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error) {
	if m.consumeFunc != nil {
		return m.consumeFunc(queue, consumer, autoAck, exclusive, noLocal, noWait, args)
	}
	return nil, nil
}

func (m *mockChannel) Qos(prefetchCount, prefetchSize int, global bool) error {
	if m.qosFunc != nil {
		return m.qosFunc(prefetchCount, prefetchSize, global)
	}
	return nil
}

func (m *mockChannel) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

func TestDefaultClientConfig(t *testing.T) {
	url := "amqp://user:pass@localhost:5672/"
	cfg := DefaultClientConfig(url)

	if cfg.URL != url {
		t.Errorf("URL = %v, want %v", cfg.URL, url)
	}
	if cfg.QueueName != "transcode_tasks" {
		t.Errorf("QueueName = %v, want %v", cfg.QueueName, "transcode_tasks")
	}
	if cfg.Exchange != "" {
		t.Errorf("Exchange = %v, want empty string", cfg.Exchange)
	}
	if cfg.RoutingKey != "transcode_tasks" {
		t.Errorf("RoutingKey = %v, want %v", cfg.RoutingKey, "transcode_tasks")
	}
	if cfg.Prefetch != 1 {
		t.Errorf("Prefetch = %v, want %v", cfg.Prefetch, 1)
	}
}

func TestClient_PublishTranscodeTask(t *testing.T) {
	tests := []struct {
		name        string
		task        repository.TranscodeTask
		mockChannel *mockChannel
		wantErr     bool
		errContains string
	}{
		{
			name: "successful publish",
			task: repository.TranscodeTask{
				VideoID:     uuid.New(),
				OriginalKey: "uploads/video-123/original.mp4",
				OutputKey:   "hls/video-123/",
			},
			mockChannel: &mockChannel{
				publishWithContextFunc: func(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error {
					// Verify message properties
					if msg.DeliveryMode != amqp.Persistent {
						t.Errorf("DeliveryMode = %v, want %v", msg.DeliveryMode, amqp.Persistent)
					}
					if msg.ContentType != "application/json" {
						t.Errorf("ContentType = %v, want %v", msg.ContentType, "application/json")
					}
					return nil
				},
			},
			wantErr: false,
		},
		{
			name: "publish error",
			task: repository.TranscodeTask{
				VideoID:     uuid.New(),
				OriginalKey: "uploads/video-123/original.mp4",
				OutputKey:   "hls/video-123/",
			},
			mockChannel: &mockChannel{
				publishWithContextFunc: func(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error {
					return errors.New("connection closed")
				},
			},
			wantErr:     true,
			errContains: "failed to publish task",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{
				channel: tt.mockChannel,
				config: ClientConfig{
					Exchange:   "",
					RoutingKey: "transcode_tasks",
				},
			}

			err := client.PublishTranscodeTask(context.Background(), tt.task)

			if (err != nil) != tt.wantErr {
				t.Errorf("PublishTranscodeTask() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.errContains != "" && err != nil {
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error = %v, should contain %v", err.Error(), tt.errContains)
				}
			}
		})
	}
}

func TestClient_PublishTranscodeTask_MessageContent(t *testing.T) {
	task := repository.TranscodeTask{
		VideoID:     uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
		OriginalKey: "uploads/video-123/original.mp4",
		OutputKey:   "hls/video-123/",
	}

	var capturedBody []byte
	mockCh := &mockChannel{
		publishWithContextFunc: func(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error {
			capturedBody = msg.Body
			return nil
		},
	}

	client := &Client{
		channel: mockCh,
		config: ClientConfig{
			Exchange:   "",
			RoutingKey: "transcode_tasks",
		},
	}

	err := client.PublishTranscodeTask(context.Background(), task)
	if err != nil {
		t.Fatalf("PublishTranscodeTask() unexpected error = %v", err)
	}

	var decoded repository.TranscodeTask
	if err := json.Unmarshal(capturedBody, &decoded); err != nil {
		t.Fatalf("failed to unmarshal captured body: %v", err)
	}

	if decoded.VideoID != task.VideoID {
		t.Errorf("VideoID = %v, want %v", decoded.VideoID, task.VideoID)
	}
	if decoded.OriginalKey != task.OriginalKey {
		t.Errorf("OriginalKey = %v, want %v", decoded.OriginalKey, task.OriginalKey)
	}
	if decoded.OutputKey != task.OutputKey {
		t.Errorf("OutputKey = %v, want %v", decoded.OutputKey, task.OutputKey)
	}
}

func TestClient_ConsumeTranscodeTasks(t *testing.T) {
	tests := []struct {
		name           string
		setupMock      func() (*mockChannel, chan amqp.Delivery)
		handler        func(task repository.TranscodeTask) error
		contextTimeout time.Duration
		wantErr        bool
		errContains    string
	}{
		{
			name: "consume registration error",
			setupMock: func() (*mockChannel, chan amqp.Delivery) {
				return &mockChannel{
					consumeFunc: func(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error) {
						return nil, errors.New("channel closed")
					},
				}, nil
			},
			handler:     func(task repository.TranscodeTask) error { return nil },
			wantErr:     true,
			errContains: "failed to register consumer",
		},
		{
			name: "context cancellation",
			setupMock: func() (*mockChannel, chan amqp.Delivery) {
				deliveries := make(chan amqp.Delivery)
				return &mockChannel{
					consumeFunc: func(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error) {
						return deliveries, nil
					},
				}, deliveries
			},
			handler:        func(task repository.TranscodeTask) error { return nil },
			contextTimeout: 50 * time.Millisecond,
			wantErr:        true,
			errContains:    "context",
		},
		{
			name: "channel closed",
			setupMock: func() (*mockChannel, chan amqp.Delivery) {
				deliveries := make(chan amqp.Delivery)
				return &mockChannel{
					consumeFunc: func(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error) {
						// Close channel immediately to simulate broker disconnect
						close(deliveries)
						return deliveries, nil
					},
				}, deliveries
			},
			handler:     func(task repository.TranscodeTask) error { return nil },
			wantErr:     true,
			errContains: "channel closed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCh, _ := tt.setupMock()
			client := &Client{
				channel: mockCh,
				config: ClientConfig{
					QueueName: "transcode_tasks",
				},
			}

			ctx := context.Background()
			if tt.contextTimeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tt.contextTimeout)
				defer cancel()
			}

			err := client.ConsumeTranscodeTasks(ctx, tt.handler)

			if (err != nil) != tt.wantErr {
				t.Errorf("ConsumeTranscodeTasks() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.errContains != "" && err != nil {
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error = %v, should contain %v", err.Error(), tt.errContains)
				}
			}
		})
	}
}

func TestClient_ConsumeTranscodeTasks_MessageHandling(t *testing.T) {
	task := repository.TranscodeTask{
		VideoID:     uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
		OriginalKey: "uploads/video-123/original.mp4",
		OutputKey:   "hls/video-123/",
	}
	taskBody, _ := json.Marshal(task)

	tests := []struct {
		name            string
		messageBody     []byte
		handlerErr      error
		expectAck       bool
		expectNack      bool
		expectRequeue   bool
		handlerReceived *repository.TranscodeTask
	}{
		{
			name:          "successful message processing",
			messageBody:   taskBody,
			handlerErr:    nil,
			expectAck:     true,
			expectNack:    false,
			expectRequeue: false,
		},
		{
			name:          "malformed JSON - nack without requeue",
			messageBody:   []byte("invalid json"),
			handlerErr:    nil,
			expectAck:     false,
			expectNack:    true,
			expectRequeue: false,
		},
		{
			name:          "handler error - nack with requeue",
			messageBody:   taskBody,
			handlerErr:    errors.New("processing failed"),
			expectAck:     false,
			expectNack:    true,
			expectRequeue: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deliveries := make(chan amqp.Delivery, 1)
			ackCalled := false
			nackCalled := false
			nackRequeue := false

			// Create a delivery with mock acknowledger
			delivery := amqp.Delivery{
				Body: tt.messageBody,
				Acknowledger: &mockAcknowledger{
					ackFunc: func(tag uint64, multiple bool) error {
						ackCalled = true
						return nil
					},
					nackFunc: func(tag uint64, multiple bool, requeue bool) error {
						nackCalled = true
						nackRequeue = requeue
						return nil
					},
				},
			}
			deliveries <- delivery

			mockCh := &mockChannel{
				consumeFunc: func(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error) {
					return deliveries, nil
				},
			}

			client := &Client{
				channel: mockCh,
				config: ClientConfig{
					QueueName: "transcode_tasks",
				},
			}

			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			var receivedTask repository.TranscodeTask
			handler := func(task repository.TranscodeTask) error {
				receivedTask = task
				return tt.handlerErr
			}

			// Run consumer (will exit on context timeout)
			_ = client.ConsumeTranscodeTasks(ctx, handler)

			// Verify acknowledgement behavior
			if tt.expectAck && !ackCalled {
				t.Error("expected Ack to be called, but it wasn't")
			}
			if !tt.expectAck && ackCalled {
				t.Error("expected Ack not to be called, but it was")
			}
			if tt.expectNack && !nackCalled {
				t.Error("expected Nack to be called, but it wasn't")
			}
			if !tt.expectNack && nackCalled {
				t.Error("expected Nack not to be called, but it was")
			}
			if tt.expectNack && tt.expectRequeue != nackRequeue {
				t.Errorf("Nack requeue = %v, want %v", nackRequeue, tt.expectRequeue)
			}

			// Verify task was correctly parsed (for valid JSON)
			if tt.expectAck || (tt.expectNack && tt.expectRequeue) {
				if receivedTask.VideoID != task.VideoID {
					t.Errorf("received VideoID = %v, want %v", receivedTask.VideoID, task.VideoID)
				}
			}
		})
	}
}

// mockAcknowledger implements amqp.Acknowledger for testing.
type mockAcknowledger struct {
	ackFunc    func(tag uint64, multiple bool) error
	nackFunc   func(tag uint64, multiple bool, requeue bool) error
	rejectFunc func(tag uint64, requeue bool) error
}

func (m *mockAcknowledger) Ack(tag uint64, multiple bool) error {
	if m.ackFunc != nil {
		return m.ackFunc(tag, multiple)
	}
	return nil
}

func (m *mockAcknowledger) Nack(tag uint64, multiple bool, requeue bool) error {
	if m.nackFunc != nil {
		return m.nackFunc(tag, multiple, requeue)
	}
	return nil
}

func (m *mockAcknowledger) Reject(tag uint64, requeue bool) error {
	if m.rejectFunc != nil {
		return m.rejectFunc(tag, requeue)
	}
	return nil
}

func TestClient_Close(t *testing.T) {
	tests := []struct {
		name        string
		mockChannel *mockChannel
		mockConn    *mockConnection
		wantErr     bool
		errContains string
	}{
		{
			name: "successful close",
			mockChannel: &mockChannel{
				closeFunc: func() error { return nil },
			},
			mockConn: &mockConnection{
				closeFunc: func() error { return nil },
			},
			wantErr: false,
		},
		{
			name: "channel close error",
			mockChannel: &mockChannel{
				closeFunc: func() error { return errors.New("channel close failed") },
			},
			mockConn: &mockConnection{
				closeFunc: func() error { return nil },
			},
			wantErr:     true,
			errContains: "failed to close channel",
		},
		{
			name: "connection close error",
			mockChannel: &mockChannel{
				closeFunc: func() error { return nil },
			},
			mockConn: &mockConnection{
				closeFunc: func() error { return errors.New("connection close failed") },
			},
			wantErr:     true,
			errContains: "failed to close connection",
		},
		{
			name: "both close errors",
			mockChannel: &mockChannel{
				closeFunc: func() error { return errors.New("channel close failed") },
			},
			mockConn: &mockConnection{
				closeFunc: func() error { return errors.New("connection close failed") },
			},
			wantErr:     true,
			errContains: "channel",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{
				conn:    tt.mockConn,
				channel: tt.mockChannel,
			}

			err := client.Close()

			if (err != nil) != tt.wantErr {
				t.Errorf("Close() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.errContains != "" && err != nil {
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error = %v, should contain %v", err.Error(), tt.errContains)
				}
			}
		})
	}
}

func TestClient_Close_NilFields(t *testing.T) {
	// Test that Close handles nil channel and connection gracefully
	client := &Client{
		conn:    nil,
		channel: nil,
	}

	err := client.Close()
	if err != nil {
		t.Errorf("Close() with nil fields should not error, got %v", err)
	}
}
