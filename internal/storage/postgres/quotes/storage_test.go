package postgres

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/Roma12344321/currencies/internal/domain"
)

type poolAdapter struct {
	*pgxpool.Pool
}

func (p poolAdapter) GetConn() *pgxpool.Pool {
	return p.Pool
}

type StorageTest struct {
	suite.Suite

	ctx       context.Context
	container *postgres.PostgresContainer
	pool      *pgxpool.Pool
	storage   *Storage
}

func TestInit(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}

	t.Parallel()
	suite.Run(t, new(StorageTest))
}

func (s *StorageTest) SetupSuite() {
	s.ctx = context.Background()

	migrationsPath, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "migrations", "01_init.sql"))
	s.Require().NoError(err)

	s.container, err = postgres.Run(s.ctx,
		"postgres:16.4-alpine",
		postgres.WithDatabase("currencies"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		postgres.WithInitScripts(migrationsPath),
	)
	s.Require().NoError(err)

	connStr, err := s.container.ConnectionString(s.ctx, "sslmode=disable")
	s.Require().NoError(err)

	s.pool, err = pgxpool.New(s.ctx, connStr)
	s.Require().NoError(err)

	s.Require().Eventually(func() bool {
		pingCtx, cancel := context.WithTimeout(s.ctx, time.Second)
		defer cancel()

		return s.pool.Ping(pingCtx) == nil
	}, 30*time.Second, 500*time.Millisecond, "postgres container did not become ready")

	s.storage = New(poolAdapter{Pool: s.pool})
}

func (s *StorageTest) TearDownSuite() {
	if s.pool != nil {
		s.pool.Close()
	}

	if s.container != nil {
		err := s.container.Terminate(s.ctx)
		s.Require().NoError(err)
	}
}

func (s *StorageTest) createRequest(pair domain.Pair) domain.QuoteRequest {
	s.T().Helper()

	request := domain.QuoteRequest{
		ID:        uuid.NewV7(),
		Pair:      pair,
		Status:    domain.RequestStatusPending,
		CreatedAt: time.Now().UTC(),
	}

	s.Require().NoError(s.storage.CreateQuoteRequest(s.ctx, request))

	return request
}

func (s *StorageTest) Test_GetActiveRequest() {
	request := s.createRequest(domain.Pair{Base: "EUR", Quote: "MXN"})

	active, err := s.storage.GetActiveRequest(s.ctx, domain.Pair{Base: "EUR", Quote: "MXN"})
	s.Require().NoError(err)
	s.Equal(request.ID, active.ID)

	s.Require().NoError(s.storage.MarkProcessed(s.ctx, request.ID, domain.RequestStatusDone))

	_, err = s.storage.GetActiveRequest(s.ctx, domain.Pair{Base: "EUR", Quote: "MXN"})
	s.ErrorIs(err, domain.ErrQuoteNotFound)
}

func (s *StorageTest) Test_ClaimMarkDoneFlow() {
	request := s.createRequest(domain.Pair{Base: "MXN", Quote: "EUR"})

	claimed, err := s.storage.ClaimPendingRequests(s.ctx, 10)
	s.Require().NoError(err)
	s.Require().Len(claimed, 1)
	s.Equal(request.ID, claimed[0].ID)
	s.Equal(domain.RequestStatusProcessing, claimed[0].Status)

	s.Require().NoError(s.storage.SaveQuote(s.ctx, domain.Quote{
		Pair:      domain.Pair{Base: "MXN", Quote: "EUR"},
		Price:     18.5,
		RequestID: request.ID,
		UpdatedAt: time.Now().UTC(),
	}))
	s.Require().NoError(s.storage.MarkProcessed(s.ctx, request.ID, domain.RequestStatusDone))

	stored, err := s.storage.GetQuoteRequest(s.ctx, request.ID)
	s.Require().NoError(err)
	s.Equal(domain.RequestStatusDone, stored.Status)

	quote, err := s.storage.GetQuoteByRequestID(s.ctx, request.ID)
	s.Require().NoError(err)
	s.Equal(domain.Pair{Base: "MXN", Quote: "EUR"}, quote.Pair)
	s.InDelta(18.5, quote.Price, 0.0001)
}

func (s *StorageTest) Test_ClaimStaleProcessing() {
	request := s.createRequest(domain.Pair{Base: "JPY", Quote: "EUR"})

	claimed, err := s.storage.ClaimPendingRequests(s.ctx, 10)
	s.Require().NoError(err)
	s.Require().Len(claimed, 1)

	_, err = s.pool.Exec(s.ctx,
		"UPDATE quote_requests SET updated_at = now() - interval '2 minutes' WHERE id = $1", request.ID)
	s.Require().NoError(err)

	reclaimed, err := s.storage.ClaimPendingRequests(s.ctx, 10)
	s.Require().NoError(err)
	s.Require().Len(reclaimed, 1)
	s.Equal(request.ID, reclaimed[0].ID)
}

func (s *StorageTest) Test_ClaimEmpty() {
	claimed, err := s.storage.ClaimPendingRequests(s.ctx, 10)
	s.Require().NoError(err)
	s.Empty(claimed)
}

func (s *StorageTest) Test_SaveQuoteUpsert() {
	request := s.createRequest(domain.Pair{Base: "GBP", Quote: "EUR"})

	save := func(price float64) {
		s.Require().NoError(s.storage.SaveQuote(s.ctx, domain.Quote{
			Pair:      domain.Pair{Base: "GBP", Quote: "EUR"},
			Price:     price,
			RequestID: request.ID,
			UpdatedAt: time.Now().UTC(),
		}))
	}

	save(1.2)
	save(1.5)

	quote, err := s.storage.GetLastQuote(s.ctx, domain.Pair{Base: "GBP", Quote: "EUR"})
	s.Require().NoError(err)
	s.InDelta(1.5, quote.Price, 0.0001)
	s.Equal(request.ID, quote.RequestID)

	var rows int
	err = s.pool.QueryRow(s.ctx, "SELECT count(*) FROM quotes WHERE pair = 'GBP/EUR'").Scan(&rows)
	s.Require().NoError(err)
	s.Equal(1, rows)
}

func (s *StorageTest) Test_NotFoundErrors() {
	_, err := s.storage.GetQuoteRequest(s.ctx, uuid.NewV7())
	s.ErrorIs(err, domain.ErrQuoteNotFound)

	_, err = s.storage.GetLastQuote(s.ctx, domain.Pair{Base: "CHF", Quote: "EUR"})
	s.ErrorIs(err, domain.ErrQuoteNotFound)

	_, err = s.storage.GetQuoteByRequestID(s.ctx, uuid.NewV7())
	s.ErrorIs(err, domain.ErrQuoteNotFound)
}

func (s *StorageTest) Test_ConcurrentClaimsDisjoint() {
	const total = 20

	all := make(map[uuid.UUID]struct{}, total)
	for i := range total {
		pair := domain.Pair{Base: "EUR", Quote: fmt.Sprintf("C%02d", i)}
		request := s.createRequest(pair)
		all[request.ID] = struct{}{}
	}

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		claimed = make(map[uuid.UUID]struct{}, total)
	)

	claimBatch := func() {
		defer wg.Done()

		requests, err := s.storage.ClaimPendingRequests(s.ctx, total/2)
		s.Require().NoError(err)

		mu.Lock()
		defer mu.Unlock()

		for _, request := range requests {
			claimed[request.ID] = struct{}{}
		}
	}

	wg.Add(2)
	go claimBatch()
	go claimBatch()
	wg.Wait()

	s.Len(claimed, total)

	for id := range all {
		s.Contains(claimed, id)
	}
}
