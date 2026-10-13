package txmanager

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/martynov-roman/currencies/internal/tracing"
)

type txKey struct{}

type ConnManager interface {
	GetConn() *pgxpool.Pool
}

type Manager struct {
	connManager ConnManager
}

func New(connManager ConnManager) *Manager {
	return &Manager{connManager: connManager}
}

func WithTx(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}

type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func GetQuerier(ctx context.Context, conn *pgxpool.Pool) Querier {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}

	return conn
}

func (m *Manager) Do(ctx context.Context, process func(ctx context.Context) error) error {
	ctx, span := otel.Tracer("txmanager.Manager").Start(ctx, "Do", trace.WithSpanKind(trace.SpanKindInternal))
	defer span.End()

	tx, err := m.connManager.GetConn().Begin(ctx)
	if err != nil {
		return tracing.AddErrorToSpan(span, fmt.Errorf("begin: %w", err))
	}

	defer func() { _ = tx.Rollback(ctx) }()

	if err = process(WithTx(ctx, tx)); err != nil {
		return tracing.AddErrorToSpan(span, err)
	}

	if err = tx.Commit(ctx); err != nil {
		return tracing.AddErrorToSpan(span, fmt.Errorf("commit: %w", err))
	}

	return nil
}
