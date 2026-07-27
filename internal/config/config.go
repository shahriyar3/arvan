package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	App         AppConfig         `mapstructure:"app"`
	HTTP        HTTPConfig        `mapstructure:"http"`
	Database    DatabaseConfig    `mapstructure:"database"`
	Redis       RedisConfig       `mapstructure:"redis"`
	RabbitMQ    RabbitMQConfig    `mapstructure:"rabbitmq"`
	OutboxRelay OutboxRelayConfig `mapstructure:"outbox_relay"`
	Worker      WorkerConfig      `mapstructure:"worker"`
	Operator    OperatorConfig    `mapstructure:"operator"`
	MockOp      MockOperatorConfig `mapstructure:"mock_operator"`
	RateLimit   RateLimitConfig   `mapstructure:"rate_limit"`
}

type AppConfig struct {
	Name        string `mapstructure:"name"`
	Environment string `mapstructure:"environment"`
	LogLevel    string `mapstructure:"log_level"`
}

type HTTPConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

type DatabaseConfig struct {
	PrimaryDSN string `mapstructure:"primary_dsn"`
	ReplicaDSN string `mapstructure:"replica_dsn"`
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type RabbitMQConfig struct {
	URL string `mapstructure:"url"`
}

type OutboxRelayConfig struct {
	PollInterval    time.Duration `mapstructure:"poll_interval"`
	BatchSize       int           `mapstructure:"batch_size"`
	LockDuration    time.Duration `mapstructure:"lock_duration"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

type WorkerConfig struct {
	PrefetchCount       int           `mapstructure:"prefetch_count"`
	ShutdownTimeout     time.Duration `mapstructure:"shutdown_timeout"`
	ExpressPoolSize     int           `mapstructure:"express_pool_size"`
	StandardPoolSize    int           `mapstructure:"standard_pool_size"`
	MaxDeliveryAttempts int           `mapstructure:"max_delivery_attempts"`
	MetricsPort         int           `mapstructure:"metrics_port"`
}

type CircuitBreakerConfig struct {
	MaxRequests uint32        `mapstructure:"max_requests"`
	Interval    time.Duration `mapstructure:"interval"`
	Timeout     time.Duration `mapstructure:"timeout"`
	ReadyToTrip uint32        `mapstructure:"consecutive_failures"`
}

type RateLimitConfig struct {
	Enabled   bool          `mapstructure:"enabled"`
	Window    time.Duration `mapstructure:"window"`
	Limit     int64         `mapstructure:"limit"`
	KeyPrefix string        `mapstructure:"key_prefix"`
}

type OperatorConfig struct {
	BaseURL        string               `mapstructure:"base_url"`
	Timeout        time.Duration        `mapstructure:"timeout"`
	CircuitBreaker CircuitBreakerConfig `mapstructure:"circuit_breaker"`
}

type MockOperatorConfig struct {
	Host         string        `mapstructure:"host"`
	Port         int           `mapstructure:"port"`
	MinLatency   time.Duration `mapstructure:"min_latency"`
	MaxLatency   time.Duration `mapstructure:"max_latency"`
	FailureRate  float64       `mapstructure:"failure_rate"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
}

func (c MockOperatorConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

func Load() (*Config, error) {
	v := viper.New()
	v.SetConfigName(".env")
	v.SetConfigType("env")
	v.AddConfigPath(".")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	setDefaults(v)
	bindEnv(v)

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("read config: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	if cfg.Database.ReplicaDSN == "" {
		cfg.Database.ReplicaDSN = cfg.Database.PrimaryDSN
	}

	return &cfg, nil
}

func bindEnv(v *viper.Viper) {
	keys := []string{
		"app.name",
		"app.environment",
		"app.log_level",
		"http.host",
		"http.port",
		"http.read_timeout",
		"http.write_timeout",
		"http.shutdown_timeout",
		"database.primary_dsn",
		"database.replica_dsn",
		"redis.addr",
		"redis.password",
		"redis.db",
		"rabbitmq.url",
		"outbox_relay.poll_interval",
		"outbox_relay.batch_size",
		"outbox_relay.lock_duration",
		"outbox_relay.shutdown_timeout",
		"worker.prefetch_count",
		"worker.shutdown_timeout",
		"worker.express_pool_size",
		"worker.standard_pool_size",
		"worker.max_delivery_attempts",
		"worker.metrics_port",
		"operator.base_url",
		"operator.timeout",
		"operator.circuit_breaker.max_requests",
		"operator.circuit_breaker.interval",
		"operator.circuit_breaker.timeout",
		"operator.circuit_breaker.consecutive_failures",
		"rate_limit.enabled",
		"rate_limit.window",
		"rate_limit.limit",
		"rate_limit.key_prefix",
		"mock_operator.host",
		"mock_operator.port",
		"mock_operator.min_latency",
		"mock_operator.max_latency",
		"mock_operator.failure_rate",
		"mock_operator.read_timeout",
		"mock_operator.write_timeout",
	}
	for _, key := range keys {
		_ = v.BindEnv(key)
	}
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("app.name", "sms-gateway")
	v.SetDefault("app.environment", "development")
	v.SetDefault("app.log_level", "info")

	v.SetDefault("http.host", "0.0.0.0")
	v.SetDefault("http.port", 8080)
	v.SetDefault("http.read_timeout", "10s")
	v.SetDefault("http.write_timeout", "10s")
	v.SetDefault("http.shutdown_timeout", "15s")

	v.SetDefault("database.primary_dsn", "postgres://sms:sms@localhost:5433/sms_gateway?sslmode=disable")
	v.SetDefault("database.replica_dsn", "")

	v.SetDefault("redis.addr", "localhost:6379")
	v.SetDefault("redis.password", "")
	v.SetDefault("redis.db", 0)

	v.SetDefault("rabbitmq.url", "amqp://sms:sms@localhost:5672/")

	v.SetDefault("outbox_relay.poll_interval", "500ms")
	v.SetDefault("outbox_relay.batch_size", 50)
	v.SetDefault("outbox_relay.lock_duration", "30s")
	v.SetDefault("outbox_relay.shutdown_timeout", "15s")

	v.SetDefault("worker.prefetch_count", 10)
	v.SetDefault("worker.shutdown_timeout", "30s")
	v.SetDefault("worker.express_pool_size", 20)
	v.SetDefault("worker.standard_pool_size", 50)
	v.SetDefault("worker.max_delivery_attempts", 5)
	v.SetDefault("worker.metrics_port", 9091)

	v.SetDefault("operator.base_url", "http://localhost:8090")
	v.SetDefault("operator.timeout", "10s")
	v.SetDefault("operator.circuit_breaker.max_requests", 3)
	v.SetDefault("operator.circuit_breaker.interval", "0s")
	v.SetDefault("operator.circuit_breaker.timeout", "30s")
	v.SetDefault("operator.circuit_breaker.consecutive_failures", 5)

	v.SetDefault("rate_limit.enabled", true)
	v.SetDefault("rate_limit.window", "1s")
	v.SetDefault("rate_limit.limit", 100)
	v.SetDefault("rate_limit.key_prefix", "ratelimit")

	v.SetDefault("mock_operator.host", "0.0.0.0")
	v.SetDefault("mock_operator.port", 8090)
	v.SetDefault("mock_operator.min_latency", "10ms")
	v.SetDefault("mock_operator.max_latency", "50ms")
	v.SetDefault("mock_operator.failure_rate", 0.0)
	v.SetDefault("mock_operator.read_timeout", "5s")
	v.SetDefault("mock_operator.write_timeout", "5s")
}

func (c HTTPConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}
