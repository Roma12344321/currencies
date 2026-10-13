package config

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/pkgerrors"
	"go.opentelemetry.io/otel/trace"
)

const formatJSON = "json"

type ContextHook struct{}

func (h ContextHook) Run(event *zerolog.Event, _ zerolog.Level, _ string) {
	ctx := event.GetCtx()
	if ctx == nil {
		return
	}

	span := trace.SpanFromContext(ctx)

	if span.SpanContext().IsValid() {
		event.Str("trace_id", span.SpanContext().TraceID().String()).
			Str("span_id", span.SpanContext().SpanID().String())
	}
}

func InitLogger(cfg Logger) zerolog.Logger {
	level := zerolog.InfoLevel
	if newLevel, err := zerolog.ParseLevel(cfg.LogLevel); err == nil {
		level = newLevel
	}

	var out io.Writer = os.Stdout
	if cfg.LogFormat != formatJSON {
		out = zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.StampMicro}
	}
	//nolint:reassign
	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack

	return zerolog.New(out).Level(level).
		With().Caller().Timestamp().
		Logger().Hook(ContextHook{})
}
