package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/Roma12344321/currencies/internal/domain"
	txmanager "github.com/Roma12344321/currencies/internal/storage/postgres/tx-manager"
	"github.com/Roma12344321/currencies/internal/tracing"
)

type ConnManager interface {
	GetConn() *pgxpool.Pool
}

const staleProcessingAge = time.Minute

type Storage struct {
	connManager ConnManager
}

func New(connManager ConnManager) *Storage {
	return &Storage{connManager: connManager}
}

func (s *Storage) CreateQuoteRequest(ctx context.Context, request domain.QuoteRequest) error {
	ctx, span := otel.Tracer("quotes.Storage").Start(
		ctx,
		"CreateQuoteRequest",
		trace.WithAttributes(
			attribute.Stringer("requestID", request.ID),
			attribute.String("currency", request.Currency),
		),
	)
	defer span.End()

	const query = `
		INSERT INTO quote_requests(id, currency, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)`

	_, err := txmanager.GetQuerier(ctx, s.connManager.GetConn()).
		Exec(ctx, query, request.ID, request.Currency, request.Status, request.CreatedAt, request.CreatedAt)
	if err != nil {
		return tracing.AddErrorToSpan(span, fmt.Errorf("storage.CreateQuoteRequest: %w", err))
	}

	return nil
}

func (s *Storage) GetQuoteRequest(ctx context.Context, requestID uuid.UUID) (domain.QuoteRequest, error) {
	ctx, span := otel.Tracer("quotes.Storage").Start(
		ctx,
		"GetQuoteRequest",
		trace.WithAttributes(attribute.Stringer("requestID", requestID)),
	)
	defer span.End()

	var request domain.QuoteRequest

	const query = `
		SELECT id, currency, status, created_at
		FROM quote_requests
		WHERE id = $1`

	err := txmanager.GetQuerier(ctx, s.connManager.GetConn()).
		QueryRow(ctx, query, requestID).
		Scan(&request.ID, &request.Currency, &request.Status, &request.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.QuoteRequest{}, domain.ErrQuoteNotFound
		}

		return domain.QuoteRequest{}, tracing.AddErrorToSpan(span, fmt.Errorf("storage.GetQuoteRequest: %w", err))
	}

	return request, nil
}

func (s *Storage) GetActiveRequest(ctx context.Context, currency string) (domain.QuoteRequest, error) {
	ctx, span := otel.Tracer("quotes.Storage").Start(
		ctx,
		"GetActiveRequest",
		trace.WithAttributes(attribute.String("currency", currency)),
	)
	defer span.End()

	var request domain.QuoteRequest

	const query = `
		SELECT id, currency, status, created_at
		FROM quote_requests
		WHERE currency = $1 AND status IN ('pending', 'processing')
		ORDER BY created_at
		LIMIT 1`

	err := txmanager.GetQuerier(ctx, s.connManager.GetConn()).
		QueryRow(ctx, query, currency).
		Scan(&request.ID, &request.Currency, &request.Status, &request.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.QuoteRequest{}, domain.ErrQuoteNotFound
		}

		return domain.QuoteRequest{}, tracing.AddErrorToSpan(span, fmt.Errorf("storage.GetActiveRequest: %w", err))
	}

	return request, nil
}

func (s *Storage) GetQuoteByRequestID(ctx context.Context, requestID uuid.UUID) (domain.Quote, error) {
	ctx, span := otel.Tracer("quotes.Storage").Start(
		ctx,
		"GetQuoteByRequestID",
		trace.WithAttributes(attribute.Stringer("requestID", requestID)),
	)
	defer span.End()

	var quote domain.Quote

	const query = `
		SELECT currency, price, request_id, updated_at
		FROM quotes
		WHERE request_id = $1`

	err := txmanager.GetQuerier(ctx, s.connManager.GetConn()).
		QueryRow(ctx, query, requestID).
		Scan(&quote.Currency, &quote.Price, &quote.RequestID, &quote.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Quote{}, domain.ErrQuoteNotFound
		}

		return domain.Quote{}, tracing.AddErrorToSpan(span, fmt.Errorf("storage.GetQuoteByRequestID: %w", err))
	}

	return quote, nil
}

func (s *Storage) MarkProcessed(ctx context.Context, requestID uuid.UUID, status domain.RequestStatus) error {
	ctx, span := otel.Tracer("quotes.Storage").Start(
		ctx,
		"MarkProcessed",
		trace.WithAttributes(
			attribute.Stringer("requestID", requestID),
			attribute.String("status", string(status)),
		),
	)
	defer span.End()

	const query = `
		UPDATE quote_requests
		SET status = $2, updated_at = $3
		WHERE id = $1 AND status IN ('pending', 'processing')`

	_, err := txmanager.GetQuerier(ctx, s.connManager.GetConn()).
		Exec(ctx, query, requestID, status, time.Now().UTC())
	if err != nil {
		return tracing.AddErrorToSpan(span, fmt.Errorf("storage.MarkProcessed: %w", err))
	}

	return nil
}

func (s *Storage) SaveQuote(ctx context.Context, quote domain.Quote) error {
	ctx, span := otel.Tracer("quotes.Storage").Start(
		ctx,
		"SaveQuote",
		trace.WithAttributes(
			attribute.String("currency", quote.Currency),
			attribute.Float64("price", quote.Price),
		),
	)
	defer span.End()

	const query = `
		INSERT INTO quotes(currency, price, request_id, updated_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (currency) DO UPDATE
		SET price = EXCLUDED.price, request_id = EXCLUDED.request_id, updated_at = EXCLUDED.updated_at`

	_, err := txmanager.GetQuerier(ctx, s.connManager.GetConn()).
		Exec(ctx, query, quote.Currency, quote.Price, quote.RequestID, quote.UpdatedAt)
	if err != nil {
		return tracing.AddErrorToSpan(span, fmt.Errorf("storage.SaveQuote: %w", err))
	}

	return nil
}

func (s *Storage) GetLastQuote(ctx context.Context, currency string) (domain.Quote, error) {
	ctx, span := otel.Tracer("quotes.Storage").Start(
		ctx,
		"GetLastQuote",
		trace.WithAttributes(attribute.String("currency", currency)),
	)
	defer span.End()

	var quote domain.Quote

	const query = `
		SELECT currency, price, request_id, updated_at
		FROM quotes
		WHERE currency = $1`

	err := txmanager.GetQuerier(ctx, s.connManager.GetConn()).
		QueryRow(ctx, query, currency).
		Scan(&quote.Currency, &quote.Price, &quote.RequestID, &quote.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Quote{}, domain.ErrQuoteNotFound
		}

		return domain.Quote{}, tracing.AddErrorToSpan(span, fmt.Errorf("storage.GetLastQuote: %w", err))
	}

	return quote, nil
}

func (s *Storage) ClaimPendingRequests(ctx context.Context, limit int) ([]domain.QuoteRequest, error) {
	ctx, span := otel.Tracer("quotes.Storage").Start(
		ctx,
		"ClaimPendingRequests",
		trace.WithAttributes(attribute.Int("limit", limit)),
	)
	defer span.End()

	const query = `
		UPDATE quote_requests
		SET status = 'processing', updated_at = $2
		WHERE id IN (
			SELECT id FROM quote_requests
			WHERE status = 'pending'
			   OR (status = 'processing' AND updated_at < $3)
			ORDER BY created_at
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, currency, status, created_at`

	rows, err := txmanager.GetQuerier(ctx, s.connManager.GetConn()).
		Query(ctx, query, limit, time.Now().UTC(), time.Now().UTC().Add(-staleProcessingAge))
	if err != nil {
		return nil, tracing.AddErrorToSpan(span, fmt.Errorf("storage.ClaimPendingRequests: %w", err))
	}
	defer rows.Close()

	requests := make([]domain.QuoteRequest, 0, limit)

	for rows.Next() {
		var request domain.QuoteRequest
		if err = rows.Scan(&request.ID, &request.Currency, &request.Status, &request.CreatedAt); err != nil {
			return nil, tracing.AddErrorToSpan(span, fmt.Errorf("scan pending request: %w", err))
		}

		requests = append(requests, request)
	}

	if err = rows.Err(); err != nil {
		return nil, tracing.AddErrorToSpan(span, fmt.Errorf("rows iterate: %w", err))
	}

	return requests, nil
}
