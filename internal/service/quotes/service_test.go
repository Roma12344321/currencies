package quotes

import (
	"context"
	"errors"
	"testing"
	"time"
	"uuid"

	"github.com/Roma12344321/currencies/internal/domain"
	"github.com/Roma12344321/currencies/internal/service/quotes/mocks"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type ServiceTest struct {
	suite.Suite

	ctrl     *gomock.Controller
	storage  *mocks.MockStorage
	exchange *mocks.MockExchangeClient
	tx       *mocks.MockTxManager

	service *Service
}

func TestInit(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(ServiceTest))
}

func (s *ServiceTest) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.storage = mocks.NewMockStorage(s.ctrl)
	s.exchange = mocks.NewMockExchangeClient(s.ctrl)
	s.tx = mocks.NewMockTxManager(s.ctrl)

	s.tx.EXPECT().
		Do(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, process func(ctx context.Context) error) error {
			return process(ctx)
		}).
		AnyTimes()

	s.service = New(zerolog.Nop(), s.storage, s.exchange, s.tx, []string{"EUR", "MXN"})
}

func (s *ServiceTest) Test_CreateQuoteRequest() {
	tt := []struct {
		name       string
		currency   string
		mocksInit  func()
		expCreated bool
		expErr     error
	}{
		{
			name:     "new request",
			currency: "EUR",
			mocksInit: func() {
				s.storage.EXPECT().
					GetActiveRequest(gomock.Any(), "EUR").
					Return(domain.QuoteRequest{}, domain.ErrQuoteNotFound)
				s.storage.EXPECT().
					CreateQuoteRequest(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, r domain.QuoteRequest) error {
						s.Require().Equal("EUR", r.Currency)
						s.Require().Equal(domain.RequestStatusPending, r.Status)
						return nil
					})
			},
			expCreated: true,
		},
		{
			name:     "idempotent",
			currency: "EUR",
			mocksInit: func() {
				s.storage.EXPECT().
					GetActiveRequest(gomock.Any(), "EUR").
					Return(domain.QuoteRequest{
						ID:       uuid.NewV7(),
						Currency: "EUR",
						Status:   domain.RequestStatusPending,
					}, nil)
			},
			expCreated: false,
		},
		{
			name:      "unsupported currency",
			currency:  "JPY",
			mocksInit: func() {},
			expErr:    domain.ErrCurrencyNotSupported,
		},
		{
			name:     "storage error",
			currency: "EUR",
			mocksInit: func() {
				s.storage.EXPECT().
					GetActiveRequest(gomock.Any(), "EUR").
					Return(domain.QuoteRequest{}, errors.New("db down"))
			},
			expErr: errors.New("db down"),
		},
	}

	for _, tc := range tt {
		s.Run(tc.name, func() {
			tc.mocksInit()

			request, created, err := s.service.CreateQuoteRequest(context.Background(), tc.currency)

			if tc.expErr != nil {
				s.ErrorContains(err, tc.expErr.Error())
				s.False(created)

				return
			}

			s.NoError(err)
			s.Require().Equal(tc.currency, request.Currency)
			s.Equal(tc.expCreated, created)
			s.NotEmpty(request.ID)
		})
	}
}

func (s *ServiceTest) Test_GetQuote() {
	requestID := uuid.NewV7()

	tt := []struct {
		name      string
		mocksInit func()
		expErr    error
	}{
		{
			name: "ready",
			mocksInit: func() {
				s.storage.EXPECT().
					GetQuoteRequest(gomock.Any(), requestID).
					Return(domain.QuoteRequest{Status: domain.RequestStatusDone}, nil)
				s.storage.EXPECT().
					GetQuoteByRequestID(gomock.Any(), requestID).
					Return(domain.Quote{Currency: "EUR", Price: 0.9}, nil)
			},
		},
		{
			name: "not ready",
			mocksInit: func() {
				s.storage.EXPECT().
					GetQuoteRequest(gomock.Any(), requestID).
					Return(domain.QuoteRequest{Status: domain.RequestStatusPending}, nil)
				s.storage.EXPECT().
					GetQuoteByRequestID(gomock.Any(), requestID).
					Return(domain.Quote{}, domain.ErrQuoteNotFound)
			},
			expErr: domain.ErrQuoteNotReady,
		},
		{
			name: "request not found",
			mocksInit: func() {
				s.storage.EXPECT().
					GetQuoteRequest(gomock.Any(), requestID).
					Return(domain.QuoteRequest{}, domain.ErrQuoteNotFound)
			},
			expErr: domain.ErrQuoteNotFound,
		},
	}

	for _, tc := range tt {
		s.Run(tc.name, func() {
			tc.mocksInit()

			quote, err := s.service.GetQuote(context.Background(), requestID)

			if tc.expErr != nil {
				s.ErrorIs(err, tc.expErr)
				return
			}

			s.NoError(err)
			s.InDelta(0.9, quote.Price, 0.0001)
		})
	}
}

func (s *ServiceTest) Test_GetLastQuote() {
	tt := []struct {
		name      string
		currency  string
		mocksInit func()
		expErr    error
	}{
		{
			name:     "found",
			currency: "MXN",
			mocksInit: func() {
				s.storage.EXPECT().
					GetLastQuote(gomock.Any(), "MXN").
					Return(domain.Quote{Currency: "MXN", Price: 18.5, UpdatedAt: time.Now()}, nil)
			},
		},
		{
			name:      "unsupported",
			currency:  "GBP",
			mocksInit: func() {},
			expErr:    domain.ErrCurrencyNotSupported,
		},
		{
			name:     "not found",
			currency: "EUR",
			mocksInit: func() {
				s.storage.EXPECT().
					GetLastQuote(gomock.Any(), "EUR").
					Return(domain.Quote{}, domain.ErrQuoteNotFound)
			},
			expErr: domain.ErrQuoteNotFound,
		},
	}

	for _, tc := range tt {
		s.Run(tc.name, func() {
			tc.mocksInit()

			quote, err := s.service.GetLastQuote(context.Background(), tc.currency)

			if tc.expErr != nil {
				s.ErrorIs(err, tc.expErr)
				return
			}

			s.NoError(err)
			s.Equal(tc.currency, quote.Currency)
		})
	}
}

func (s *ServiceTest) Test_ProcessPending() {
	requests := []domain.QuoteRequest{
		{ID: uuid.NewV7(), Currency: "EUR"},
		{ID: uuid.NewV7(), Currency: "MXN"},
	}

	tt := []struct {
		name      string
		mocksInit func()
		expErr    bool
	}{
		{
			name: "saves quotes and marks done",
			mocksInit: func() {
				s.storage.EXPECT().
					ClaimPendingRequests(gomock.Any(), 100).
					Return(requests, nil)
				s.exchange.EXPECT().
					LatestRates(gomock.Any(), []string{"EUR", "MXN"}).
					Return(map[string]float64{"EUR": 0.9, "MXN": 18.5}, nil)

				for _, request := range requests {
					s.storage.EXPECT().
						SaveQuote(gomock.Any(), gomock.Any()).
						Do(func(_ context.Context, q domain.Quote) {
							s.Require().Equal(request.Currency, q.Currency)
							s.Require().Equal(request.ID, q.RequestID)
						}).
						Return(nil)
					s.storage.EXPECT().
						MarkProcessed(gomock.Any(), request.ID, domain.RequestStatusDone)
				}
			},
		},
		{
			name: "exchange unavailable releases to pending",
			mocksInit: func() {
				s.storage.EXPECT().
					ClaimPendingRequests(gomock.Any(), 100).
					Return(requests, nil)
				s.exchange.EXPECT().
					LatestRates(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("connection refused"))
				for _, request := range requests {
					s.storage.EXPECT().
						MarkProcessed(gomock.Any(), request.ID, domain.RequestStatusPending)
				}
			},
			expErr: true,
		},
		{
			name: "no pending requests",
			mocksInit: func() {
				s.storage.EXPECT().
					ClaimPendingRequests(gomock.Any(), 100).
					Return(nil, nil)
			},
		},
	}

	for _, tc := range tt {
		s.Run(tc.name, func() {
			tc.mocksInit()

			err := s.service.ProcessPending(context.Background(), 100)

			if tc.expErr {
				s.Error(err)
				return
			}

			s.NoError(err)
		})
	}
}
