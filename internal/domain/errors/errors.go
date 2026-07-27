package errors

import "errors"

var (
	ErrUnauthorized        = errors.New("unauthorized")
	ErrInsufficientBalance = errors.New("insufficient balance")
	ErrNotFound            = errors.New("not found")
	ErrInvalidAmount       = errors.New("invalid amount")
	ErrInvalidCursor       = errors.New("invalid cursor")
)
