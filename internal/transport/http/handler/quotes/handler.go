//go:generate mockgen -source=./handler.go -destination=./mocks/mocks.go -package=mocks
//nolint:wrapcheck,varnamelen
package quotes

import (
	"context"
	"errors"
	"net/http"
	"time"
	"uuid"

	"github.com/labstack/echo/v5"

	"github.com/Roma12344321/currencies/internal/domain"
	"github.com/Roma12344321/currencies/internal/transport/http/common"
)

var (
	errInvalidCurrency  = errors.New("invalid currency")
	errInvalidRequestID = errors.New("invalid request id")
)

type Service interface {
	CreateQuoteRequest(ctx context.Context, currency string) (domain.QuoteRequest, bool, error)
	GetQuote(ctx context.Context, requestID uuid.UUID) (domain.Quote, error)
	GetLastQuote(ctx context.Context, currency string) (domain.Quote, error)
}

type Handler struct {
	service Service
}

func New(service Service) *Handler {
	return &Handler{service: service}
}

type CreateQuoteRequestResponse struct {
	RequestID string    `json:"requestID"`
	Status    string    `json:"status"`
	Created   bool      `json:"created"`
	CreatedAt time.Time `json:"createdAt"`
}

type QuoteResponse struct {
	Currency  string    `json:"currency"`
	Price     float64   `json:"price"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// @Summary      Обновить котировку
// @Description  Регистрирует асинхронный запрос обновления котировки валюты и возвращает его идентификатор.
// @Description  Повторный вызов с той же валютой при незавершённом запросе возвращает существующий идентификатор.
// @Tags         quotes
// @Produce      json
// @Param        currency  query  string  true  "Код валюты (EUR, MXN)"  example(EUR)
// @Success      202  {object}  common.Response{data=CreateQuoteRequestResponse} "Запрос принят"
// @Failure      400  {object}  common.Response  "Некорректный код валюты"
// @Failure      500  {object}  common.Response  "Внутренняя ошибка сервера"
// @Router       /api/v1/quotes [post]
func (h *Handler) CreateQuoteRequest(c *echo.Context) error {
	currency := c.QueryParam("currency")

	if currency == "" {
		return common.BadRequestError(errInvalidCurrency)
	}

	request, created, err := h.service.CreateQuoteRequest(c.Request().Context(), currency)
	if err != nil {
		if errors.Is(err, domain.ErrCurrencyNotSupported) {
			return common.BadRequestError(err)
		}

		return common.InternalServerError(err)
	}

	return c.JSON(http.StatusAccepted, common.Response{Data: CreateQuoteRequestResponse{
		RequestID: request.ID.String(),
		Status:    string(request.Status),
		Created:   created,
		CreatedAt: request.CreatedAt,
	}})
}

// @Summary      Получить котировку по идентификатору
// @Description  Возвращает значение цены и время обновления по идентификатору запроса.
// @Description  Если обработка ещё не завершена — 404 с соответствующей ошибкой.
// @Tags         quotes
// @Produce      json
// @Param        requestID  path  string  true  "Идентификатор запроса"  example(0192f4a0-0000-7000-8000-000000000000)
// @Success      200  {object}  common.Response{data=QuoteResponse} "Значение котировки"
// @Failure      400  {object}  common.Response  "Некорректный идентификатор"
// @Failure      404  {object}  common.Response  "Запрос не найден или котировка не готова"
// @Failure      500  {object}  common.Response  "Внутренняя ошибка сервера"
// @Router       /api/v1/quotes/{requestID} [get]
func (h *Handler) GetQuote(c *echo.Context) error {
	requestID, err := uuid.Parse(c.Param("requestID"))
	if err != nil {
		return common.BadRequestError(errInvalidRequestID)
	}

	quote, err := h.service.GetQuote(c.Request().Context(), requestID)
	if err != nil {
		if errors.Is(err, domain.ErrQuoteNotFound) || errors.Is(err, domain.ErrQuoteNotReady) {
			return common.NotFoundError(err)
		}

		return common.InternalServerError(err)
	}

	return c.JSON(http.StatusOK, common.Response{Data: QuoteResponse{
		Currency:  quote.Currency,
		Price:     quote.Price,
		UpdatedAt: quote.UpdatedAt,
	}})
}

// @Summary      Получить последнее значение котировки
// @Description  Возвращает последнее сохранённое значение цены и время обновления по коду валюты
// @Tags         quotes
// @Produce      json
// @Param        currency  query  string  true  "Код валюты (EUR, MXN)"  example(EUR)
// @Success      200  {object}  common.Response{data=QuoteResponse} "Значение котировки"
// @Failure      400  {object}  common.Response  "Некорректный код валюты"
// @Failure      404  {object}  common.Response  "Котировка не найдена"
// @Failure      500  {object}  common.Response  "Внутренняя ошибка сервера"
// @Router       /api/v1/quotes/last [get]
func (h *Handler) GetLastQuote(c *echo.Context) error {
	currency := c.QueryParam("currency")

	if currency == "" {
		return common.BadRequestError(errInvalidCurrency)
	}

	quote, err := h.service.GetLastQuote(c.Request().Context(), currency)
	if err != nil {
		if errors.Is(err, domain.ErrCurrencyNotSupported) {
			return common.BadRequestError(err)
		}

		if errors.Is(err, domain.ErrQuoteNotFound) {
			return common.NotFoundError(err)
		}

		return common.InternalServerError(err)
	}

	return c.JSON(http.StatusOK, common.Response{Data: QuoteResponse{
		Currency:  quote.Currency,
		Price:     quote.Price,
		UpdatedAt: quote.UpdatedAt,
	}})
}
