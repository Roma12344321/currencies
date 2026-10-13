package middleware

import (
	"fmt"
	"runtime/debug"

	"github.com/labstack/echo/v5"

	"github.com/Roma12344321/currencies/internal/transport/http/common"
)

func RecoverMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(ctx *echo.Context) (returnErr error) {
		defer func() {
			if err := recover(); err != nil {
				errWithStack := fmt.Errorf("panic: %v, stack trace %s", err, debug.Stack())
				returnErr = common.InternalServerError(errWithStack)
			}
		}()

		return next(ctx)
	}
}
