package operator

import (
	"context"
)

type SendRequest struct {
	MessageID string
	AccountID string
	To        string
	Body      string
}

type SendResult struct {
	OperatorRef string
}

type SMSOperator interface {
	Send(ctx context.Context, req SendRequest) (SendResult, error)
}
