package domain

import (
	"errors"
	"time"
	"uuid"
)

var (
	ErrCurrencyNotSupported = errors.New("currency not supported")
	ErrQuoteNotFound        = errors.New("quote not found")
	ErrQuoteNotReady        = errors.New("quote is not ready yet")
)

type RequestStatus string

const (
	RequestStatusPending    RequestStatus = "pending"
	RequestStatusProcessing RequestStatus = "processing"
	RequestStatusDone       RequestStatus = "done"
	RequestStatusFailed     RequestStatus = "failed"
)

type QuoteRequest struct {
	ID        uuid.UUID
	Currency  string
	Status    RequestStatus
	CreatedAt time.Time
}

type Quote struct {
	Currency  string
	Price     float64
	UpdatedAt time.Time
	RequestID uuid.UUID
}
