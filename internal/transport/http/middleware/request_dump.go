package middleware

import (
	"errors"

	"github.com/labstack/echo/v5"
	"github.com/rs/zerolog"

	"github.com/martynov-roman/currencies/internal/transport/http/common"
)

type RequestDump struct {
	logger zerolog.Logger
}

func NewRequestDump(logger zerolog.Logger) *RequestDump {
	return &RequestDump{logger: logger.With().Str("component", "request_dump").Logger()}
}

//nolint:varnamelen
func (m *RequestDump) Middleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			err := next(c)
			if err == nil {
				return nil
			}

			var httpError *echo.HTTPError
			if !errors.As(err, &httpError) {
				return err
			}

			reqCtx := c.Request().Context()

			internalErr := httpError.Unwrap()
			if internalErr == nil {
				internalErr = httpError
			}

			requestID := c.Response().Header().Get(echo.HeaderXRequestID)
			if requestID == "" {
				requestID = c.Request().Header.Get(echo.HeaderXRequestID)
			}

			m.logger.Error().
				Ctx(reqCtx).
				Err(internalErr).
				Str("method", c.Request().Method).
				Str("path", c.Request().URL.Path).
				Int("status", httpError.Code).
				Str("ip", c.RealIP()).
				Str("user_agent", c.Request().UserAgent()).
				Str("x-request-id", requestID).
				Msg("HTTP request failed")

			return c.JSON(httpError.Code, common.Response{
				Error:     true,
				ErrorText: httpError.Message,
			})
		}
	}
}
