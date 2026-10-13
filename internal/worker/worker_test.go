package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Roma12344321/currencies/internal/worker/mocks"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type WorkerTest struct {
	suite.Suite

	ctrl     *gomock.Controller
	service  *mocks.MockQuotesProcessor
	interval time.Duration
	batch    int

	worker *Worker
}

func TestInit(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(WorkerTest))
}

func (s *WorkerTest) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.service = mocks.NewMockQuotesProcessor(s.ctrl)
	s.interval = 10 * time.Millisecond
	s.batch = 100

	s.worker = New(zerolog.Nop(), s.service, s.interval, s.batch)
}

func (s *WorkerTest) Test_RunProcessesPeriodically() {
	s.service.EXPECT().
		ProcessPending(gomock.Any(), s.batch).
		Return(nil).
		MinTimes(3)

	ctx, cancel := context.WithTimeout(context.Background(), 3*s.interval+5*time.Millisecond)
	defer cancel()

	s.NoError(s.worker.Run(ctx))
}

func (s *WorkerTest) Test_RunContinuesAfterError() {
	s.service.EXPECT().
		ProcessPending(gomock.Any(), s.batch).
		Return(errors.New("db down")).
		MinTimes(2)

	ctx, cancel := context.WithTimeout(context.Background(), 2*s.interval+5*time.Millisecond)
	defer cancel()

	s.NoError(s.worker.Run(ctx))
}

func (s *WorkerTest) Test_RunStopsOnCancel() {
	s.service.EXPECT().
		ProcessPending(gomock.Any(), s.batch).
		Return(nil).
		AnyTimes()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	s.NoError(s.worker.Run(ctx))
}
