package common

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

type Response struct {
	Data      any    `json:"data"`
	Error     bool   `json:"error"`
	ErrorText string `json:"errorText"`
}

func newHTTPError(code int, text string) *echo.HTTPError {
	return echo.NewHTTPError(code, text)
}

func BadRequestError(err error) error {
	return newHTTPError(http.StatusBadRequest, "invalid request").Wrap(err)
}

func NotFoundError(err error) error {
	return newHTTPError(http.StatusNotFound, "not found").Wrap(err)
}

func ConflictError(err error) error {
	return newHTTPError(http.StatusConflict, "conflict error").Wrap(err)
}

func InternalServerError(err error) error {
	return newHTTPError(http.StatusInternalServerError, "internal server error").Wrap(err)
}
