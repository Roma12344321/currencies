//go:generate mockgen -source=./service.go -destination=./mocks/mocks.go -package=mocks
package quotes

import (
	"context"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Roma12344321/currencies/internal/domain"
	"github.com/Roma12344321/currencies/internal/tracing"
)

type ExchangeClient interface {
	LatestRates(ctx context.Context, base string, quotes []string) (map[string]float64, error)
}

type Storage interface {
	CreateQuoteRequest(ctx context.Context, request domain.QuoteRequest) error
	GetQuoteRequest(ctx context.Context, requestID uuid.UUID) (domain.QuoteRequest, error)
	GetActiveRequest(ctx context.Context, pair domain.Pair) (domain.QuoteRequest, error)
	GetQuoteByRequestID(ctx context.Context, requestID uuid.UUID) (domain.Quote, error)
	MarkProcessed(ctx context.Context, requestID uuid.UUID, status domain.RequestStatus) error
	SaveQuote(ctx context.Context, quote domain.Quote) error
	GetLastQuote(ctx context.Context, pair domain.Pair) (domain.Quote, error)
	ClaimPendingRequests(ctx context.Context, limit int) ([]domain.QuoteRequest, error)
}

type TxManager interface {
	Do(ctx context.Context, process func(ctx context.Context) error) error
}

type Service struct {
	logger    zerolog.Logger
	storage   Storage
	exchange  ExchangeClient
	txManager TxManager
	supported map[string]struct{}
}

func New(
	logger zerolog.Logger,
	storage Storage,
	exchange ExchangeClient,
	txManager TxManager,
	supportedCurrencies []string,
) *Service {
	supported := make(map[string]struct{}, len(supportedCurrencies))
	for _, curr := range supportedCurrencies {
		supported[curr] = struct{}{}
	}

	return &Service{
		logger:    logger.With().Str("component", "quotes.Service").Logger(),
		storage:   storage,
		exchange:  exchange,
		txManager: txManager,
		supported: supported,
	}
}

func (s *Service) pairSupported(pair domain.Pair) bool {
	_, baseOK := s.supported[pair.Base]
	_, quoteOK := s.supported[pair.Quote]

	return baseOK && quoteOK
}

func (s *Service) CreateQuoteRequest(
	ctx context.Context,
	pair domain.Pair,
) (domain.QuoteRequest, bool, error) {
	ctx, span := otel.Tracer("quotes.Service").Start(
		ctx,
		"CreateQuoteRequest",
		trace.WithAttributes(attribute.String("pair", pair.String())),
	)
	defer span.End()

	if !s.pairSupported(pair) {
		return domain.QuoteRequest{}, false, tracing.AddErrorToSpan(span, domain.ErrPairNotSupported)
	}

	var (
		request  domain.QuoteRequest
		created  bool
		innerErr error
	)

	innerErr = s.txManager.Do(ctx, func(ctx context.Context) error {
		request, created, innerErr = s.createInsideTx(ctx, pair)
		return innerErr
	})

	if innerErr != nil {
		return domain.QuoteRequest{}, false, tracing.AddErrorToSpan(span, innerErr)
	}

	span.SetAttributes(attribute.Bool("created", created))
	span.SetStatus(codes.Ok, "ok")

	return request, created, nil
}

func (s *Service) createInsideTx(ctx context.Context, pair domain.Pair) (domain.QuoteRequest, bool, error) {
	pending, err := s.storage.GetActiveRequest(ctx, pair)
	if err != nil && !errors.Is(err, domain.ErrQuoteNotFound) {
		return domain.QuoteRequest{}, false, fmt.Errorf("get active request: %w", err)
	}

	if err == nil {
		return pending, false, nil
	}

	request := domain.QuoteRequest{
		ID:        uuid.NewV7(),
		Pair:      pair,
		Status:    domain.RequestStatusPending,
		CreatedAt: time.Now().UTC(),
	}

	if err = s.storage.CreateQuoteRequest(ctx, request); err != nil {
		return domain.QuoteRequest{}, false, fmt.Errorf("create quote request: %w", err)
	}

	return request, true, nil
}

func (s *Service) GetQuote(ctx context.Context, requestID uuid.UUID) (domain.Quote, error) {
	ctx, span := otel.Tracer("quotes.Service").Start(
		ctx,
		"GetQuote",
		trace.WithAttributes(attribute.Stringer("requestID", requestID)),
	)
	defer span.End()

	request, err := s.storage.GetQuoteRequest(ctx, requestID)
	if err != nil {
		return domain.Quote{}, tracing.AddErrorToSpan(span, err)
	}

	quote, err := s.storage.GetQuoteByRequestID(ctx, requestID)
	if err != nil {
		if errors.Is(err, domain.ErrQuoteNotFound) {
			return domain.Quote{}, tracing.AddErrorToSpan(span,
				fmt.Errorf("%w: request status %s", domain.ErrQuoteNotReady, request.Status))
		}

		return domain.Quote{}, tracing.AddErrorToSpan(span, err)
	}

	return quote, nil
}

func (s *Service) GetLastQuote(ctx context.Context, pair domain.Pair) (domain.Quote, error) {
	ctx, span := otel.Tracer("quotes.Service").Start(
		ctx,
		"GetLastQuote",
		trace.WithAttributes(attribute.String("pair", pair.String())),
	)
	defer span.End()

	if !s.pairSupported(pair) {
		return domain.Quote{}, tracing.AddErrorToSpan(span, domain.ErrPairNotSupported)
	}

	quote, err := s.storage.GetLastQuote(ctx, pair)
	if err != nil {
		return domain.Quote{}, tracing.AddErrorToSpan(span, err)
	}

	return quote, nil
}

func (s *Service) ProcessPending(ctx context.Context, batch int) error {
	ctx, span := otel.Tracer("quotes.Service").Start(
		ctx,
		"ProcessPending",
		trace.WithSpanKind(trace.SpanKindConsumer),
	)
	defer span.End()

	requests, err := s.storage.ClaimPendingRequests(ctx, batch)
	if err != nil {
		return tracing.AddErrorToSpan(span, err)
	}

	if len(requests) == 0 {
		return nil
	}

	err = s.processRequests(ctx, requests)

	span.SetAttributes(attribute.Int("processed", len(requests)))

	if err != nil {
		return tracing.AddErrorToSpan(span, err)
	}

	span.SetStatus(codes.Ok, "ok")

	return nil
}

func (s *Service) processRequests(ctx context.Context, requests []domain.QuoteRequest) error {
	groups := make(map[string][]domain.QuoteRequest, len(requests))
	for _, request := range requests {
		groups[request.Pair.Base] = append(groups[request.Pair.Base], request)
	}

	var processingErr error

	for base, group := range groups {
		if err := s.processGroup(ctx, base, group); err != nil {
			processingErr = errors.Join(processingErr, err)
		}
	}

	return processingErr
}

func (s *Service) processGroup(ctx context.Context, base string, group []domain.QuoteRequest) error {
	quotes := make([]string, 0, len(group))
	for _, request := range group {
		quotes = append(quotes, request.Pair.Quote)
	}

	rates, err := s.exchange.LatestRates(ctx, base, quotes)
	if err != nil {
		s.releaseGroup(ctx, group)

		return fmt.Errorf("exchange rates for base %s: %w", base, err)
	}

	for _, request := range group {
		price, ok := rates[request.Pair.Quote]
		if !ok {
			s.markRequest(ctx, request, domain.RequestStatusFailed)

			continue
		}

		errTx := s.txManager.Do(ctx, func(ctx context.Context) error {
			if err := s.storage.SaveQuote(ctx, domain.Quote{
				Pair:      request.Pair,
				Price:     price,
				RequestID: request.ID,
				UpdatedAt: time.Now().UTC(),
			}); err != nil {
				return fmt.Errorf("save quote: %w", err)
			}

			if err := s.storage.MarkProcessed(ctx, request.ID, domain.RequestStatusDone); err != nil {
				return fmt.Errorf("mark processed: %w", err)
			}

			return nil
		})
		if errTx != nil {
			s.logger.Error().Ctx(ctx).Err(errTx).Str("requestID", request.ID.String()).Msg("failed to process quote request")
			_ = s.storage.MarkProcessed(ctx, request.ID, domain.RequestStatusFailed)

			continue
		}

		s.logger.Info().Ctx(ctx).
			Str("requestID", request.ID.String()).
			Str("pair", request.Pair.String()).
			Float64("price", price).
			Msg("quote updated")
	}

	return nil
}

func (s *Service) releaseGroup(ctx context.Context, group []domain.QuoteRequest) {
	for _, request := range group {
		_ = s.storage.MarkProcessed(ctx, request.ID, domain.RequestStatusPending)
	}
}

func (s *Service) markRequest(ctx context.Context, request domain.QuoteRequest, status domain.RequestStatus) {
	if err := s.storage.MarkProcessed(ctx, request.ID, status); err != nil {
		s.logger.Error().Ctx(ctx).Err(err).Str("requestID", request.ID.String()).Msg("failed to mark request")
	}
}
