package config

import (
	"time"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	Logger  Logger
	Tracing struct {
		OtelEndpoint string `envconfig:"OTEL_ENDPOINT" default:"localhost:4318"`
		ServiceName  string `envconfig:"OTEL_SERVICE_NAME" default:"currencies"`
	}
	Server struct {
		ServerBind  string `envconfig:"SERVER_BIND" default:":8080"`
		MetricsBind string `envconfig:"BIND_METRICS" default:":8081"`
		HealthBind  string `envconfig:"BIND_HEALTH" default:":8082"`
		ServerName  string `envconfig:"SERVER_NAME" default:"currencies"`
	}
	PostgresConn struct {
		MaxConns          int           `envconfig:"POSTGRES_MAX_CONNS" default:"25"`
		MinConns          int           `envconfig:"POSTGRES_MIN_CONNS" default:"5"`
		MaxConnIdleTime   time.Duration `envconfig:"POSTGRES_MAX_CONN_IDLE_TIME" default:"5m"`
		MaxConnLifetime   time.Duration `envconfig:"POSTGRES_MAX_CONN_LIFE_TIME" default:"1h"`
		HealthCheckPeriod time.Duration `envconfig:"POSTGRES_HEALTH_CHECK_PERIOD" default:"1m"`
	}
	Worker struct {
		Interval time.Duration `envconfig:"WORKER_INTERVAL" default:"5s"`
		Batch    int           `envconfig:"WORKER_BATCH" default:"100"`
	}
	Exchange struct {
		BaseURL       string        `envconfig:"EXCHANGE_BASE_URL" default:"https://api.frankfurter.dev/v1"`
		Timeout       time.Duration `envconfig:"EXCHANGE_TIMEOUT" default:"10s"`
		SupportedList []string      `envconfig:"EXCHANGE_SUPPORTED" default:"EUR,MXN"`
	}
	Postgres Postgres
}

type Logger struct {
	LogLevel  string `envconfig:"LOGGER_LEVEL" default:"debug"`
	LogFormat string `envconfig:"LOGGER_FORMAT" default:"console"`
}

type Postgres struct {
	Host     string `envconfig:"POSTGRES_HOST" required:"true"`
	Port     int    `envconfig:"POSTGRES_PORT" required:"true"`
	DBName   string `envconfig:"POSTGRES_DB_NAME" required:"true"`
	User     string `envconfig:"POSTGRES_USER" required:"true"`
	Password string `envconfig:"POSTGRES_PASSWORD" required:"true"`
}

//nolint:wrapcheck
func Parse() (*Config, error) {
	cfg := &Config{}

	err := envconfig.Process("", cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}
