package quotes

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"uuid"

	"github.com/Roma12344321/currencies/internal/domain"
	mockquotes "github.com/Roma12344321/currencies/internal/transport/http/handler/quotes/mocks"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type HandlerTest struct {
	suite.Suite

	e       *echo.Echo
	ctrl    *gomock.Controller
	service *mockquotes.MockService

	handler *Handler
}

func TestInit(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(HandlerTest))
}

func (s *HandlerTest) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.service = mockquotes.NewMockService(s.ctrl)
	s.handler = New(s.service)

	s.e = echo.New()
	s.e.POST("/api/v1/quotes", s.handler.CreateQuoteRequest)
	s.e.GET("/api/v1/quotes/last", s.handler.GetLastQuote)
	s.e.GET("/api/v1/quotes/:requestID", s.handler.GetQuote)
}

func (s *HandlerTest) do(method, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	rec := httptest.NewRecorder()

	s.e.ServeHTTP(rec, req)

	return rec
}

func (s *HandlerTest) decode(rec *httptest.ResponseRecorder) map[string]any {
	var body struct {
		Data map[string]any `json:"data"`
	}

	s.Require().NoError(json.NewDecoder(rec.Body).Decode(&body))

	return body.Data
}

func (s *HandlerTest) Test_CreateQuoteRequest() {
	tt := []struct {
		name       string
		target     string
		mocksInit  func()
		expCode    int
		expCreated bool
	}{
		{
			name:   "created",
			target: "/api/v1/quotes?currency=EUR",
			mocksInit: func() {
				s.service.EXPECT().
					CreateQuoteRequest(gomock.Any(), "EUR").
					Return(domain.QuoteRequest{
						ID:        uuid.NewV7(),
						Currency:  "EUR",
						Status:    domain.RequestStatusPending,
						CreatedAt: time.Now(),
					}, true, nil)
			},
			expCode:    http.StatusAccepted,
			expCreated: true,
		},
		{
			name:   "idempotent",
			target: "/api/v1/quotes?currency=EUR",
			mocksInit: func() {
				s.service.EXPECT().
					CreateQuoteRequest(gomock.Any(), "EUR").
					Return(domain.QuoteRequest{
						ID:       uuid.NewV7(),
						Currency: "EUR",
						Status:   domain.RequestStatusPending,
					}, false, nil)
			},
			expCode:    http.StatusAccepted,
			expCreated: false,
		},
		{
			name:   "unsupported currency",
			target: "/api/v1/quotes?currency=JPY",
			mocksInit: func() {
				s.service.EXPECT().
					CreateQuoteRequest(gomock.Any(), "JPY").
					Return(domain.QuoteRequest{}, false, domain.ErrCurrencyNotSupported)
			},
			expCode: http.StatusBadRequest,
		},
		{
			name:      "empty currency",
			target:    "/api/v1/quotes",
			mocksInit: func() {},
			expCode:   http.StatusBadRequest,
		},
	}

	for _, tc := range tt {
		s.Run(tc.name, func() {
			tc.mocksInit()

			rec := s.do(http.MethodPost, tc.target)

			s.Require().Equal(tc.expCode, rec.Code)

			if tc.expCode != http.StatusAccepted {
				return
			}

			data := s.decode(rec)
			s.Equal(tc.expCreated, data["created"])
			s.NotEmpty(data["requestID"])
		})
	}
}

func (s *HandlerTest) Test_GetQuote() {
	requestID := uuid.NewV7()

	tt := []struct {
		name      string
		target    string
		mocksInit func()
		expCode   int
	}{
		{
			name:   "found",
			target: "/api/v1/quotes/" + requestID.String(),
			mocksInit: func() {
				s.service.EXPECT().
					GetQuote(gomock.Any(), requestID).
					Return(domain.Quote{Currency: "EUR", Price: 0.9, UpdatedAt: time.Now()}, nil)
			},
			expCode: http.StatusOK,
		},
		{
			name:   "not ready",
			target: "/api/v1/quotes/" + requestID.String(),
			mocksInit: func() {
				s.service.EXPECT().
					GetQuote(gomock.Any(), requestID).
					Return(domain.Quote{}, domain.ErrQuoteNotReady)
			},
			expCode: http.StatusNotFound,
		},
		{
			name:      "invalid uuid",
			target:    "/api/v1/quotes/not-a-uuid",
			mocksInit: func() {},
			expCode:   http.StatusBadRequest,
		},
		{
			name:   "internal error",
			target: "/api/v1/quotes/" + requestID.String(),
			mocksInit: func() {
				s.service.EXPECT().
					GetQuote(gomock.Any(), requestID).
					Return(domain.Quote{}, errors.New("db down"))
			},
			expCode: http.StatusInternalServerError,
		},
	}

	for _, tc := range tt {
		s.Run(tc.name, func() {
			tc.mocksInit()

			rec := s.do(http.MethodGet, tc.target)

			s.Require().Equal(tc.expCode, rec.Code)

			if tc.expCode == http.StatusOK {
				data := s.decode(rec)
				s.Equal("EUR", data["currency"])
			}
		})
	}
}

func (s *HandlerTest) Test_GetLastQuote() {
	tt := []struct {
		name      string
		target    string
		mocksInit func()
		expCode   int
	}{
		{
			name:   "found",
			target: "/api/v1/quotes/last?currency=MXN",
			mocksInit: func() {
				s.service.EXPECT().
					GetLastQuote(gomock.Any(), "MXN").
					Return(domain.Quote{Currency: "MXN", Price: 18.5, UpdatedAt: time.Now()}, nil)
			},
			expCode: http.StatusOK,
		},
		{
			name:      "empty currency",
			target:    "/api/v1/quotes/last",
			mocksInit: func() {},
			expCode:   http.StatusBadRequest,
		},
		{
			name:   "not found",
			target: "/api/v1/quotes/last?currency=EUR",
			mocksInit: func() {
				s.service.EXPECT().
					GetLastQuote(gomock.Any(), "EUR").
					Return(domain.Quote{}, domain.ErrQuoteNotFound)
			},
			expCode: http.StatusNotFound,
		},
		{
			name:   "unsupported",
			target: "/api/v1/quotes/last?currency=GBP",
			mocksInit: func() {
				s.service.EXPECT().
					GetLastQuote(gomock.Any(), "GBP").
					Return(domain.Quote{}, domain.ErrCurrencyNotSupported)
			},
			expCode: http.StatusBadRequest,
		},
	}

	for _, tc := range tt {
		s.Run(tc.name, func() {
			tc.mocksInit()

			rec := s.do(http.MethodGet, tc.target)

			s.Require().Equal(tc.expCode, rec.Code)

			if tc.expCode == http.StatusOK {
				data := s.decode(rec)
				s.Equal("MXN", data["currency"])
			}
		})
	}
}
