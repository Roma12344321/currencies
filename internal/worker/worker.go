//go:generate mockgen -source=./worker.go -destination=./mocks/mocks.go -package=mocks
package worker

import (
	"context"
	"time"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/Roma12344321/currencies/internal/tracing"
)

type QuotesProcessor interface {
	ProcessPending(ctx context.Context, batch int) error
}

type Worker struct {
	logger   zerolog.Logger
	service  QuotesProcessor
	interval time.Duration
	batch    int
}

func New(logger zerolog.Logger, service QuotesProcessor, interval time.Duration, batch int) *Worker {
	return &Worker{
		logger:   logger.With().Str("component", "worker.Worker").Logger(),
		service:  service,
		interval: interval,
		batch:    batch,
	}
}

func (w *Worker) Run(ctx context.Context) error {
	w.logger.Info().Dur("interval", w.interval).Msg("Starting quotes worker")

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			w.processTick(ctx)
		}
	}
}

func (w *Worker) processTick(ctx context.Context) {
	ctx, span := otel.Tracer("worker.Worker").Start(ctx, "processTick", trace.WithSpanKind(trace.SpanKindConsumer))
	defer span.End()

	if err := w.service.ProcessPending(ctx, w.batch); err != nil {
		_ = tracing.AddErrorToSpan(span, err)
		w.logger.Error().Ctx(ctx).Err(err).Msg("worker iteration failed")
	}
}
