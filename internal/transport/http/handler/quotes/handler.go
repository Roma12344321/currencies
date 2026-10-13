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
	errInvalidPair      = errors.New("invalid pair")
	errInvalidRequestID = errors.New("invalid request id")
)

type Service interface {
	CreateQuoteRequest(ctx context.Context, pair domain.Pair) (domain.QuoteRequest, bool, error)
	GetQuote(ctx context.Context, requestID uuid.UUID) (domain.Quote, error)
	GetLastQuote(ctx context.Context, pair domain.Pair) (domain.Quote, error)
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
	Pair      string    `json:"pair"`
	Price     float64   `json:"price"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// @Summary      Обновить котировку
// @Description  Регистрирует асинхронный запрос обновления котировки валютной пары и возвращает его идентификатор.
// @Description  Повторный вызов с той же парой при незавершённом запросе возвращает существующий идентификатор.
// @Tags         quotes
// @Produce      json
// @Param        pair  query  string  true  "Валютная пара BASE/QUOTE"  example(EUR/MXN)
// @Success      202   {object}  common.Response{data=CreateQuoteRequestResponse} "Запрос принят"
// @Failure      400   {object}  common.Response  "Некорректная или неподдерживаемая пара"
// @Failure      500   {object}  common.Response  "Внутренняя ошибка сервера"
// @Router       /api/v1/quotes [post]
func (h *Handler) CreateQuoteRequest(c *echo.Context) error {
	pair, err := domain.ParsePair(c.QueryParam("pair"))
	if err != nil {
		return common.BadRequestError(errInvalidPair)
	}

	request, created, err := h.service.CreateQuoteRequest(c.Request().Context(), pair)
	if err != nil {
		if errors.Is(err, domain.ErrPairNotSupported) {
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
		Pair:      quote.Pair.String(),
		Price:     quote.Price,
		UpdatedAt: quote.UpdatedAt,
	}})
}

// @Summary      Получить последнее значение котировки
// @Description  Возвращает последнее сохранённое значение цены и время обновления по валютной паре
// @Tags         quotes
// @Produce      json
// @Param        pair  query  string  true  "Валютная пара BASE/QUOTE"  example(EUR/MXN)
// @Success      200   {object}  common.Response{data=QuoteResponse} "Значение котировки"
// @Failure      400   {object}  common.Response  "Некорректная или неподдерживаемая пара"
// @Failure      404   {object}  common.Response  "Котировка не найдена"
// @Failure      500   {object}  common.Response  "Внутренняя ошибка сервера"
// @Router       /api/v1/quotes/last [get]
func (h *Handler) GetLastQuote(c *echo.Context) error {
	pair, err := domain.ParsePair(c.QueryParam("pair"))
	if err != nil {
		return common.BadRequestError(errInvalidPair)
	}

	quote, err := h.service.GetLastQuote(c.Request().Context(), pair)
	if err != nil {
		if errors.Is(err, domain.ErrPairNotSupported) {
			return common.BadRequestError(err)
		}

		if errors.Is(err, domain.ErrQuoteNotFound) {
			return common.NotFoundError(err)
		}

		return common.InternalServerError(err)
	}

	return c.JSON(http.StatusOK, common.Response{Data: QuoteResponse{
		Pair:      quote.Pair.String(),
		Price:     quote.Price,
		UpdatedAt: quote.UpdatedAt,
	}})
}
