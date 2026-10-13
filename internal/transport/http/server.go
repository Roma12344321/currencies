package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo-contrib/v5/echoprometheus"
	echootel "github.com/labstack/echo-opentelemetry"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	httpSwagger "github.com/swaggo/http-swagger/v2"

	_ "github.com/martynov-roman/currencies/docs"
	"github.com/martynov-roman/currencies/internal/transport/http/handler/quotes"
	customMw "github.com/martynov-roman/currencies/internal/transport/http/middleware"
)

// @title           Currencies API
// @version         1.0
// @description     Сервис асинхронных котировок валютных курсов.
// @BasePath        /
type Server struct {
	*echo.Echo

	requestDump   *customMw.RequestDump
	quotesHandler *quotes.Handler
}

func New(requestDump *customMw.RequestDump) *Server {
	return &Server{
		Echo: echo.New(),

		requestDump: requestDump,
	}
}

func (s *Server) WithSwagger() *Server {
	s.GET("/swagger/*", echo.WrapHandler(httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	)))

	return s
}

func (s *Server) WithQuotesHandler(quotesHandler *quotes.Handler) *Server {
	s.quotesHandler = quotesHandler

	return s
}

func (s *Server) Start(ctx context.Context, addr string, serverName string, shutdownTimeout time.Duration) error {
	s.Use(echootel.NewMiddleware(serverName))
	s.Use(echoprometheus.NewMiddleware(serverName))

	s.Use(middleware.RequestID())
	s.Use(s.requestDump.Middleware())
	s.Use(customMw.RecoverMiddleware)

	apiV1Group := s.Group("/api/v1")

	quotesGroup := apiV1Group.Group("/quotes")
	{
		quotesGroup.POST("", s.quotesHandler.CreateQuoteRequest)
		quotesGroup.GET("/last", s.quotesHandler.GetLastQuote)
		quotesGroup.GET("/:requestID", s.quotesHandler.GetQuote)
	}

	startConfig := echo.StartConfig{
		Address:         addr,
		GracefulTimeout: shutdownTimeout,
		HideBanner:      true,
		HidePort:        true,
	}

	if err := startConfig.Start(ctx, s); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("sc.Start: %w", err)
	}

	return nil
}
