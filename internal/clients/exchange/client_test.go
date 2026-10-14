package exchange

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type ClientTest struct {
	suite.Suite

	server *httptest.Server
	client *Client
}

func TestInit(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(ClientTest))
}

func (s *ClientTest) SetupTest() {
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Require().Equal("/latest", r.URL.Path)
		s.Require().Equal("EUR", r.URL.Query().Get("base"))
		s.Require().Equal("MXN,USD", r.URL.Query().Get("symbols"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"base":"EUR","date":"2026-10-13","rates":{"MXN":20.1,"USD":1.09}}`))
	}))

	s.client = New(s.server.URL, 5*time.Second)
}

func (s *ClientTest) TearDownTest() {
	s.server.Close()
}

func (s *ClientTest) Test_LatestRates() {
	rates, err := s.client.LatestRates(context.Background(), "EUR", []string{"MXN", "USD"})
	s.Require().NoError(err)
	s.InDelta(20.1, rates["MXN"], 0.0001)
	s.InDelta(1.09, rates["USD"], 0.0001)
}

func (s *ClientTest) Test_Non200() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	client := New(server.URL, 5*time.Second)

	_, err := client.LatestRates(context.Background(), "EUR", []string{"MXN"})
	s.ErrorContains(err, "unexpected status code 502")
}
