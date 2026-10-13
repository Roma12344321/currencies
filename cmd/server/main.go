package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo-contrib/v5/echoprometheus"
	"github.com/labstack/echo/v5"
	"golang.org/x/sync/errgroup"

	"github.com/Roma12344321/currencies/internal/clients/exchange"
	"github.com/Roma12344321/currencies/internal/config"
	quotesService "github.com/Roma12344321/currencies/internal/service/quotes"
	connManager "github.com/Roma12344321/currencies/internal/storage/postgres/conn-manager"
	quotesStorage "github.com/Roma12344321/currencies/internal/storage/postgres/quotes"
	txmanager "github.com/Roma12344321/currencies/internal/storage/postgres/tx-manager"
	"github.com/Roma12344321/currencies/internal/tracing"
	server "github.com/Roma12344321/currencies/internal/transport/http"
	quotesHandler "github.com/Roma12344321/currencies/internal/transport/http/handler/quotes"
	"github.com/Roma12344321/currencies/internal/transport/http/middleware"
	"github.com/Roma12344321/currencies/internal/worker"
)

const shutdownTimeout = 15 * time.Second

//nolint:cyclop
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Parse()
	if err != nil {
		panic(err)
	}

	logger := config.InitLogger(cfg.Logger)

	tracer, err := tracing.InitTracer(ctx, cfg)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to init tracer")
	}
	defer func() { _ = tracer.Shutdown(context.Background()) }()

	connMng, err := connManager.New(cfg)
	if err != nil {
		logger.Fatal().Err(err).Msg("could not create connection manager")
	}
	defer connMng.Close()

	txManager := txmanager.New(connMng)
	quotesSt := quotesStorage.New(connMng)

	exchangeClient := exchange.New(cfg.Exchange.BaseURL, cfg.Exchange.Timeout)
	quotesSvc := quotesService.New(logger, quotesSt, exchangeClient, txManager, cfg.Exchange.SupportedList)

	quotesWorker := worker.New(logger, quotesSvc, cfg.Worker.Interval, cfg.Worker.Batch)

	quotesHndlr := quotesHandler.New(quotesSvc)

	requestDump := middleware.NewRequestDump(logger)

	srv := server.New(requestDump).
		WithQuotesHandler(quotesHndlr).
		WithSwagger()

	metricsServer := echo.New()
	metricsServer.GET("/metrics", echoprometheus.NewHandler())

	healthServer := echo.New()
	healthServer.GET("/health", func(c *echo.Context) error {
		return c.JSON(http.StatusOK, "ok")
	})

	errG, gCtx := errgroup.WithContext(ctx)

	errG.Go(func() error {
		logger.Info().Msg("Starting server at " + cfg.Server.ServerBind)

		if err := srv.Start(gCtx, cfg.Server.ServerBind, cfg.Server.ServerName, shutdownTimeout); err != nil {
			return fmt.Errorf("server error: %w", err)
		}

		return nil
	})

	errG.Go(func() error {
		logger.Info().Msg("Starting metrics server at " + cfg.Server.MetricsBind)

		startConfig := echo.StartConfig{
			Address:         cfg.Server.MetricsBind,
			GracefulTimeout: shutdownTimeout,
			HideBanner:      true,
			HidePort:        true,
		}

		if err := startConfig.Start(gCtx, metricsServer); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("metrics server error: %w", err)
		}

		return nil
	})

	errG.Go(func() error {
		startConfig := echo.StartConfig{
			Address:         cfg.Server.HealthBind,
			GracefulTimeout: shutdownTimeout,
			HideBanner:      true,
			HidePort:        true,
		}

		logger.Info().Msg("Starting health server at " + cfg.Server.HealthBind)

		if err := startConfig.Start(gCtx, healthServer); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("health server error: %w", err)
		}

		return nil
	})

	errG.Go(func() error {
		return quotesWorker.Run(gCtx)
	})

	if err = errG.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error().Err(err).Msg("error during server execution")
	}

	logger.Info().Msg("all services stopped gracefully")
}
