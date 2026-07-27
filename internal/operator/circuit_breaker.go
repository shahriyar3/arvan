package operator

import (
	"context"
	"errors"
	"fmt"

	"github.com/sony/gobreaker"
)

var ErrCircuitOpen = errors.New("operator circuit breaker open")

type CircuitBreakerClient struct {
	inner SMSOperator
	cb    *gobreaker.CircuitBreaker
}

func NewCircuitBreakerClient(inner SMSOperator, cfg gobreaker.Settings) *CircuitBreakerClient {
	if cfg.Name == "" {
		cfg.Name = "operator"
	}
	return &CircuitBreakerClient{
		inner: inner,
		cb:    gobreaker.NewCircuitBreaker(cfg),
	}
}

func (c *CircuitBreakerClient) Send(ctx context.Context, req SendRequest) (SendResult, error) {
	result, err := c.cb.Execute(func() (any, error) {
		return c.inner.Send(ctx, req)
	})
	if err != nil {
		if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
			return SendResult{}, fmt.Errorf("%w: %v", ErrCircuitOpen, err)
		}
		return SendResult{}, err
	}

	sendResult, ok := result.(SendResult)
	if !ok {
		return SendResult{}, fmt.Errorf("unexpected operator result type")
	}
	return sendResult, nil
}

func (c *CircuitBreakerClient) State() gobreaker.State {
	return c.cb.State()
}

func IsCircuitOpen(err error) bool {
	return errors.Is(err, ErrCircuitOpen)
}
