package conn_manager

import (
	"context"
	"fmt"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/Roma12344321/currencies/internal/config"
	"github.com/Roma12344321/currencies/internal/stats"
)

type ConnManager struct {
	conn *pgxpool.Pool
}

func New(cfg *config.Config) (*ConnManager, error) {
	ctx := context.Background()

	pgConfig, err := pgxpool.ParseConfig(fmt.Sprintf(
		"host=%s port=%d dbname=%s sslmode=disable user=%s password=%s",
		cfg.Postgres.Host, cfg.Postgres.Port, cfg.Postgres.DBName, cfg.Postgres.User, cfg.Postgres.Password))
	if err != nil {
		return nil, fmt.Errorf("failed to parse dsn: %w", err)
	}

	pgConfig.ConnConfig.Tracer = otelpgx.NewTracer()

	pgConfig.MaxConns = int32(cfg.PostgresConn.MaxConns)
	pgConfig.MinConns = int32(cfg.PostgresConn.MinConns)
	pgConfig.MaxConnIdleTime = cfg.PostgresConn.MaxConnIdleTime
	pgConfig.MaxConnLifetime = cfg.PostgresConn.MaxConnLifetime
	pgConfig.HealthCheckPeriod = cfg.PostgresConn.HealthCheckPeriod

	pool, err := pgxpool.NewWithConfig(ctx, pgConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to pgxpool: %w", err)
	}

	if err = pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping db: %w", err)
	}

	prometheus.MustRegister(
		stats.NewStatsCollector(fmt.Sprintf("%s:%d", cfg.Postgres.Host, cfg.Postgres.Port), cfg.Postgres.DBName, pool),
	)

	return &ConnManager{conn: pool}, nil
}

func (m *ConnManager) GetConn() *pgxpool.Pool {
	return m.conn
}

func (m *ConnManager) Close() {
	m.conn.Close()
}
