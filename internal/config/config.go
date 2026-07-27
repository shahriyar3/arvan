package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	App      AppConfig      `mapstructure:"app"`
	HTTP     HTTPConfig     `mapstructure:"http"`
	Database DatabaseConfig `mapstructure:"database"`
	Redis    RedisConfig    `mapstructure:"redis"`
	RabbitMQ RabbitMQConfig `mapstructure:"rabbitmq"`
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
}

func (c HTTPConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}
