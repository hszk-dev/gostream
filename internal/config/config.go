package config

import (
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	Server   ServerConfig
	Worker   WorkerConfig
	Database DatabaseConfig
	MinIO    MinIOConfig
	RabbitMQ RabbitMQConfig
}

type ServerConfig struct {
	Port            int           `envconfig:"API_PORT" default:"8080"`
	ReadTimeout     time.Duration `envconfig:"API_READ_TIMEOUT" default:"10s"`
	WriteTimeout    time.Duration `envconfig:"API_WRITE_TIMEOUT" default:"30s"`
	ShutdownTimeout time.Duration `envconfig:"API_SHUTDOWN_TIMEOUT" default:"10s"`
}

type WorkerConfig struct {
	TempDir         string        `envconfig:"WORKER_TEMP_DIR" default:"/tmp/gostream"`
	MaxRetries      int           `envconfig:"WORKER_MAX_RETRIES" default:"3"`
	ShutdownTimeout time.Duration `envconfig:"WORKER_SHUTDOWN_TIMEOUT" default:"30s"`
}

type DatabaseConfig struct {
	Host     string `envconfig:"POSTGRES_HOST" default:"localhost"`
	Port     int    `envconfig:"POSTGRES_PORT" default:"5432"`
	User     string `envconfig:"POSTGRES_USER" default:"gostream"`
	Password string `envconfig:"POSTGRES_PASSWORD" default:"gostream"`
	DBName   string `envconfig:"POSTGRES_DB" default:"gostream"`
	SSLMode  string `envconfig:"POSTGRES_SSLMODE" default:"disable"`
}

func (c DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.User, c.Password, c.Host, c.Port, c.DBName, c.SSLMode,
	)
}

type MinIOConfig struct {
	Endpoint  string `envconfig:"MINIO_ENDPOINT" default:"localhost:9000"`
	AccessKey string `envconfig:"MINIO_ACCESS_KEY" default:"minioadmin"`
	SecretKey string `envconfig:"MINIO_SECRET_KEY" default:"minioadmin"`
	Bucket    string `envconfig:"MINIO_BUCKET" default:"videos"`
	UseSSL    bool   `envconfig:"MINIO_USE_SSL" default:"false"`
}

type RabbitMQConfig struct {
	Host     string `envconfig:"RABBITMQ_HOST" default:"localhost"`
	Port     int    `envconfig:"RABBITMQ_PORT" default:"5672"`
	User     string `envconfig:"RABBITMQ_USER" default:"gostream"`
	Password string `envconfig:"RABBITMQ_PASSWORD" default:"gostream"`
	VHost    string `envconfig:"RABBITMQ_VHOST" default:"/"`
}

func (c RabbitMQConfig) URL() string {
	return fmt.Sprintf(
		"amqp://%s:%s@%s:%d%s",
		c.User, c.Password, c.Host, c.Port, c.VHost,
	)
}

func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	return &cfg, nil
}
