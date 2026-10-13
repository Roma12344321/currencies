package quotes

import (
	"context"
	"errors"
	"testing"
	"time"
	"uuid"

	"github.com/rs/zerolog"

	"github.com/martynov-roman/currencies/internal/domain"
)

type storageMock struct {
	getActive   func(ctx context.Context, currency string) (domain.QuoteRequest, error)
	createReq   func(ctx context.Context, request domain.QuoteRequest) error
	getByReqID  func(ctx context.Context, requestID uuid.UUID) (domain.Quote, error)
	lastQuote   func(ctx context.Context, currency string) (domain.Quote, error)
	claim       func(ctx context.Context, limit int) ([]domain.QuoteRequest, error)
	mark        func(ctx context.Context, requestID uuid.UUID, status domain.RequestStatus) error
	save        func(ctx context.Context, quote domain.Quote) error
}

func (m *storageMock) CreateQuoteRequest(ctx context.Context, r domain.QuoteRequest) error {
	if m.createReq != nil {
		return m.createReq(ctx, r)
	}

	return nil
}

func (m *storageMock) GetQuoteRequest(_ context.Context, _ uuid.UUID) (domain.QuoteRequest, error) {
	return domain.QuoteRequest{}, nil
}

func (m *storageMock) GetActiveRequest(ctx context.Context, curr string) (domain.QuoteRequest, error) {
	return m.getActive(ctx, curr)
}

func (m *storageMock) GetQuoteByRequestID(ctx context.Context, id uuid.UUID) (domain.Quote, error) {
	return m.getByReqID(ctx, id)
}

func (m *storageMock) MarkProcessed(ctx context.Context, id uuid.UUID, st domain.RequestStatus) error {
	if m.mark != nil {
		return m.mark(ctx, id, st)
	}

	return nil
}

func (m *storageMock) SaveQuote(ctx context.Context, q domain.Quote) error {
	if m.save != nil {
		return m.save(ctx, q)
	}

	return nil
}

func (m *storageMock) GetLastQuote(ctx context.Context, curr string) (domain.Quote, error) {
	return m.lastQuote(ctx, curr)
}

func (m *storageMock) ClaimPendingRequests(ctx context.Context, limit int) ([]domain.QuoteRequest, error) {
	return m.claim(ctx, limit)
}

type exchangeMock struct {
	rates func(ctx context.Context, currencies []string) (map[string]float64, error)
}

func (m *exchangeMock) LatestRates(ctx context.Context, curr []string) (map[string]float64, error) {
	return m.rates(ctx, curr)
}

type txManagerMock struct{}

func (txManagerMock) Do(ctx context.Context, process func(ctx context.Context) error) error {
	return process(ctx)
}

var supported = []string{"EUR", "MXN"}

func newService(st storageIface, ex exchangeIface) *Service {
	return New(zerolog.Nop(), st, ex, txManagerMock{}, supported)
}

type storageIface = Storage
type exchangeIface = ExchangeClient

func TestCreateQuoteRequestNew(t *testing.T) {
	t.Parallel()

	st := &storageMock{
		getActive: func(_ context.Context, _ string) (domain.QuoteRequest, error) {
			return domain.QuoteRequest{}, domain.ErrQuoteNotFound
		},
	}
	svc := newService(st, &exchangeMock{})

	request, created, err := svc.CreateQuoteRequest(context.Background(), "EUR")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !created {
		t.Fatal("expected created=true for fresh currency")
	}

	if request.Currency != "EUR" || request.Status != domain.RequestStatusPending {
		t.Fatalf("unexpected request: %+v", request)
	}
}

func TestCreateQuoteRequestIdempotent(t *testing.T) {
	t.Parallel()

	existing := domain.QuoteRequest{ID: uuid.NewV7(), Currency: "EUR", Status: domain.RequestStatusPending}
	st := &storageMock{
		getActive: func(_ context.Context, currency string) (domain.QuoteRequest, error) {
			if currency != "EUR" {
				t.Errorf("unexpected currency: %s", currency)
			}

			return existing, nil
		},
	}
	svc := newService(st, &exchangeMock{})

	request, created, err := svc.CreateQuoteRequest(context.Background(), "EUR")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if created {
		t.Fatal("expected created=false for existing pending request")
	}

	if request.ID != existing.ID {
		t.Fatalf("expected existing request %s, got %s", existing.ID, request.ID)
	}
}

func TestCreateQuoteRequestUnsupported(t *testing.T) {
	t.Parallel()

	svc := newService(&storageMock{}, &exchangeMock{})

	_, _, err := svc.CreateQuoteRequest(context.Background(), "JPY")
	if !errors.Is(err, domain.ErrCurrencyNotSupported) {
		t.Fatalf("expected ErrCurrencyNotSupported, got %v", err)
	}
}

func TestGetQuoteNotReady(t *testing.T) {
	t.Parallel()

	requestID := uuid.NewV7()
	st := &storageMock{
		getByReqID: func(_ context.Context, _ uuid.UUID) (domain.Quote, error) {
			return domain.Quote{}, domain.ErrQuoteNotFound
		},
	}
	svc := newService(st, &exchangeMock{})

	_, err := svc.GetQuote(context.Background(), requestID)
	if !errors.Is(err, domain.ErrQuoteNotReady) {
		t.Fatalf("expected ErrQuoteNotReady, got %v", err)
	}
}

func TestProcessPendingSavesQuote(t *testing.T) {
	t.Parallel()

	requestID := uuid.NewV7()
	requests := []domain.QuoteRequest{{ID: requestID, Currency: "EUR", Status: domain.RequestStatusProcessing}}

	var saved domain.Quote

	st := &storageMock{
		claim: func(_ context.Context, _ int) ([]domain.QuoteRequest, error) {
			return requests, nil
		},
		save: func(_ context.Context, q domain.Quote) error {
			saved = q
			return nil
		},
	}
	ex := &exchangeMock{
		rates: func(_ context.Context, currencies []string) (map[string]float64, error) {
			return map[string]float64{"EUR": 0.92}, nil
		},
	}
	svc := newService(st, ex)

	if err := svc.ProcessPending(context.Background(), 10); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if saved.Currency != "EUR" || saved.Price != 0.92 || saved.RequestID != requestID {
		t.Fatalf("unexpected saved quote: %+v", saved)
	}

	if saved.UpdatedAt.IsZero() {
		t.Fatal("expected UpdatedAt to be set")
	}
}

func TestProcessPendingReleasesOnExchangeError(t *testing.T) {
	t.Parallel()

	requestID := uuid.NewV7()
	var markedStatus domain.RequestStatus

	st := &storageMock{
		claim: func(_ context.Context, _ int) ([]domain.QuoteRequest, error) {
			return []domain.QuoteRequest{{ID: requestID, Currency: "EUR"}}, nil
		},
		mark: func(_ context.Context, _ uuid.UUID, status domain.RequestStatus) error {
			markedStatus = status
			return nil
		},
	}
	ex := &exchangeMock{
		rates: func(_ context.Context, _ []string) (map[string]float64, error) {
			return nil, errors.New("connection refused")
		},
	}
	svc := newService(st, ex)

	err := svc.ProcessPending(context.Background(), 10)
	if err == nil {
		t.Fatal("expected error when exchange unavailable")
	}

	if markedStatus != domain.RequestStatusPending {
		t.Fatalf("expected request released to pending, got %s", markedStatus)
	}
}

func TestGetLastQuote(t *testing.T) {
	t.Parallel()

	updatedAt := time.Now().UTC()
	st := &storageMock{
		lastQuote: func(_ context.Context, currency string) (domain.Quote, error) {
			return domain.Quote{Currency: currency, Price: 18.5, UpdatedAt: updatedAt}, nil
		},
	}
	svc := newService(st, &exchangeMock{})

	quote, err := svc.GetLastQuote(context.Background(), "MXN")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if quote.Price != 18.5 || quote.UpdatedAt != updatedAt {
		t.Fatalf("unexpected quote: %+v", quote)
	}
}
