package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"uuid"
)

var (
	ErrPairNotSupported = errors.New("pair not supported")
	ErrInvalidPair      = errors.New("invalid pair")
	ErrQuoteNotFound    = errors.New("quote not found")
	ErrQuoteNotReady    = errors.New("quote is not ready yet")
)

type RequestStatus string

const (
	RequestStatusPending    RequestStatus = "pending"
	RequestStatusProcessing RequestStatus = "processing"
	RequestStatusDone       RequestStatus = "done"
	RequestStatusFailed     RequestStatus = "failed"
)

const currencyCodeLen = 3

type Pair struct {
	Base  string
	Quote string
}

func ParsePair(raw string) (Pair, error) {
	base, quote, ok := strings.Cut(strings.ToUpper(raw), "/")
	if !ok ||
		len(base) != currencyCodeLen || len(quote) != currencyCodeLen ||
		base == quote {
		return Pair{}, fmt.Errorf("%w: %q, want BASE/QUOTE, e.g. EUR/MXN", ErrInvalidPair, raw)
	}

	return Pair{Base: base, Quote: quote}, nil
}

func (p Pair) String() string {
	return p.Base + "/" + p.Quote
}

type QuoteRequest struct {
	ID        uuid.UUID
	Pair      Pair
	Status    RequestStatus
	CreatedAt time.Time
}

type Quote struct {
	Pair      Pair
	Price     float64
	UpdatedAt time.Time
	RequestID uuid.UUID
}
