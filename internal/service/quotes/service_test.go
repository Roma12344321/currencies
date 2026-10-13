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
		pair       domain.Pair
		mocksInit  func()
		expCreated bool
		expErr     error
	}{
		{
			name: "new request",
			pair: domain.Pair{Base: "EUR", Quote: "MXN"},
			mocksInit: func() {
				s.storage.EXPECT().
					GetActiveRequest(gomock.Any(), domain.Pair{Base: "EUR", Quote: "MXN"}).
					Return(domain.QuoteRequest{}, domain.ErrQuoteNotFound)
				s.storage.EXPECT().
					CreateQuoteRequest(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, r domain.QuoteRequest) error {
						s.Require().Equal(domain.Pair{Base: "EUR", Quote: "MXN"}, r.Pair)
						s.Require().Equal(domain.RequestStatusPending, r.Status)
						return nil
					})
			},
			expCreated: true,
		},
		{
			name: "idempotent",
			pair: domain.Pair{Base: "EUR", Quote: "MXN"},
			mocksInit: func() {
				s.storage.EXPECT().
					GetActiveRequest(gomock.Any(), domain.Pair{Base: "EUR", Quote: "MXN"}).
					Return(domain.QuoteRequest{
						ID:     uuid.NewV7(),
						Pair:   domain.Pair{Base: "EUR", Quote: "MXN"},
						Status: domain.RequestStatusPending,
					}, nil)
			},
			expCreated: false,
		},
		{
			name:      "base not supported",
			pair:      domain.Pair{Base: "JPY", Quote: "EUR"},
			mocksInit: func() {},
			expErr:    domain.ErrPairNotSupported,
		},
		{
			name:      "quote not supported",
			pair:      domain.Pair{Base: "EUR", Quote: "JPY"},
			mocksInit: func() {},
			expErr:    domain.ErrPairNotSupported,
		},
		{
			name: "storage error",
			pair: domain.Pair{Base: "EUR", Quote: "MXN"},
			mocksInit: func() {
				s.storage.EXPECT().
					GetActiveRequest(gomock.Any(), domain.Pair{Base: "EUR", Quote: "MXN"}).
					Return(domain.QuoteRequest{}, errors.New("db down"))
			},
			expErr: errors.New("db down"),
		},
	}

	for _, tc := range tt {
		s.Run(tc.name, func() {
			tc.mocksInit()

			request, created, err := s.service.CreateQuoteRequest(context.Background(), tc.pair)

			if tc.expErr != nil {
				s.ErrorContains(err, tc.expErr.Error())
				s.False(created)

				return
			}

			s.NoError(err)
			s.Require().Equal(tc.pair, request.Pair)
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
					Return(domain.Quote{Pair: domain.Pair{Base: "EUR", Quote: "MXN"}, Price: 18.5}, nil)
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
			s.InDelta(18.5, quote.Price, 0.0001)
		})
	}
}

func (s *ServiceTest) Test_GetLastQuote() {
	tt := []struct {
		name      string
		pair      domain.Pair
		mocksInit func()
		expErr    error
	}{
		{
			name: "found",
			pair: domain.Pair{Base: "EUR", Quote: "MXN"},
			mocksInit: func() {
				s.storage.EXPECT().
					GetLastQuote(gomock.Any(), domain.Pair{Base: "EUR", Quote: "MXN"}).
					Return(domain.Quote{
						Pair:      domain.Pair{Base: "EUR", Quote: "MXN"},
						Price:     18.5,
						UpdatedAt: time.Now(),
					}, nil)
			},
		},
		{
			name:      "not supported",
			pair:      domain.Pair{Base: "MXN", Quote: "JPY"},
			mocksInit: func() {},
			expErr:    domain.ErrPairNotSupported,
		},
		{
			name: "not found",
			pair: domain.Pair{Base: "EUR", Quote: "MXN"},
			mocksInit: func() {
				s.storage.EXPECT().
					GetLastQuote(gomock.Any(), domain.Pair{Base: "EUR", Quote: "MXN"}).
					Return(domain.Quote{}, domain.ErrQuoteNotFound)
			},
			expErr: domain.ErrQuoteNotFound,
		},
	}

	for _, tc := range tt {
		s.Run(tc.name, func() {
			tc.mocksInit()

			quote, err := s.service.GetLastQuote(context.Background(), tc.pair)

			if tc.expErr != nil {
				s.ErrorIs(err, tc.expErr)
				return
			}

			s.NoError(err)
			s.Equal(tc.pair, quote.Pair)
		})
	}
}

func (s *ServiceTest) Test_ProcessPending() {
	eurMxn := domain.QuoteRequest{ID: uuid.NewV7(), Pair: domain.Pair{Base: "EUR", Quote: "MXN"}}
	mxnEur := domain.QuoteRequest{ID: uuid.NewV7(), Pair: domain.Pair{Base: "MXN", Quote: "EUR"}}

	tt := []struct {
		name      string
		requests  []domain.QuoteRequest
		mocksInit func()
		expErr    bool
	}{
		{
			name:     "groups by base and saves quotes",
			requests: []domain.QuoteRequest{eurMxn, mxnEur},
			mocksInit: func() {
				s.storage.EXPECT().
					ClaimPendingRequests(gomock.Any(), 100).
					Return([]domain.QuoteRequest{eurMxn, mxnEur}, nil)

				s.exchange.EXPECT().
					LatestRates(gomock.Any(), "EUR", []string{"MXN"}).
					Return(map[string]float64{"MXN": 18.5}, nil)
				s.exchange.EXPECT().
					LatestRates(gomock.Any(), "MXN", []string{"EUR"}).
					Return(map[string]float64{"EUR": 0.054}, nil)

				for _, request := range []domain.QuoteRequest{eurMxn, mxnEur} {
					s.storage.EXPECT().
						SaveQuote(gomock.Any(), gomock.Any()).
						Do(func(_ context.Context, q domain.Quote) {
							s.Require().Equal(request.Pair, q.Pair)
							s.Require().Equal(request.ID, q.RequestID)
						}).
						Return(nil)
					s.storage.EXPECT().
						MarkProcessed(gomock.Any(), request.ID, domain.RequestStatusDone)
				}
			},
		},
		{
			name:     "failed base releases only its group",
			requests: []domain.QuoteRequest{eurMxn, mxnEur},
			mocksInit: func() {
				s.storage.EXPECT().
					ClaimPendingRequests(gomock.Any(), 100).
					Return([]domain.QuoteRequest{eurMxn, mxnEur}, nil)

				s.exchange.EXPECT().
					LatestRates(gomock.Any(), "EUR", []string{"MXN"}).
					Return(nil, errors.New("connection refused"))
				s.exchange.EXPECT().
					LatestRates(gomock.Any(), "MXN", []string{"EUR"}).
					Return(map[string]float64{"EUR": 0.054}, nil)

				s.storage.EXPECT().
					MarkProcessed(gomock.Any(), eurMxn.ID, domain.RequestStatusPending)
				s.storage.EXPECT().
					SaveQuote(gomock.Any(), gomock.Any()).
					Do(func(_ context.Context, q domain.Quote) {
						s.Require().Equal(mxnEur.Pair, q.Pair)
					}).
					Return(nil)
				s.storage.EXPECT().
					MarkProcessed(gomock.Any(), mxnEur.ID, domain.RequestStatusDone)
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
