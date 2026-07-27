package operator

import (
	"context"
	"errors"
	"testing"

	"github.com/sony/gobreaker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubOperator struct {
	err error
}

func (s stubOperator) Send(context.Context, SendRequest) (SendResult, error) {
	if s.err != nil {
		return SendResult{}, s.err
	}
	return SendResult{OperatorRef: "ref"}, nil
}

func TestCircuitBreakerClientOpensAfterFailures(t *testing.T) {
	client := NewCircuitBreakerClient(stubOperator{err: errors.New("boom")}, gobreaker.Settings{
		Name:        "test",
		MaxRequests: 1,
		Timeout:     0,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 2
		},
	})

	_, err := client.Send(context.Background(), SendRequest{})
	require.Error(t, err)
	assert.False(t, IsCircuitOpen(err))

	_, err = client.Send(context.Background(), SendRequest{})
	require.Error(t, err)
	assert.False(t, IsCircuitOpen(err))

	_, err = client.Send(context.Background(), SendRequest{})
	require.Error(t, err)
	assert.True(t, IsCircuitOpen(err))
}

func TestCircuitBreakerClientSuccess(t *testing.T) {
	client := NewCircuitBreakerClient(stubOperator{}, gobreaker.Settings{Name: "test"})
	result, err := client.Send(context.Background(), SendRequest{})
	require.NoError(t, err)
	assert.Equal(t, "ref", result.OperatorRef)
}
