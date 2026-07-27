package errors

import "errors"

var (
	ErrUnauthorized        = errors.New("unauthorized")
	ErrInsufficientBalance = errors.New("insufficient balance")
	ErrNotFound            = errors.New("not found")
	ErrInvalidAmount       = errors.New("invalid amount")
	ErrInvalidCursor       = errors.New("invalid cursor")
	ErrInvalidRequest      = errors.New("invalid request")
	ErrInvalidMessageType  = errors.New("invalid message type")
	ErrInvalidIdempotencyKey   = errors.New("invalid idempotency key")
	ErrIdempotencyInProgress   = errors.New("idempotency request in progress")
	ErrInvalidPhoneNumber  = errors.New("invalid phone number")
	ErrMessageTooLong      = errors.New("message too long")
)
