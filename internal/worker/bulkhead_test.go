package worker

import (
	"context"
	"testing"
	"time"

	"github.com/shahriyar/arvan/internal/broker"
	"github.com/stretchr/testify/require"
)

func TestBulkheadLimitsConcurrency(t *testing.T) {
	b := NewBulkhead(1, 1)
	ctx := context.Background()

	require.NoError(t, b.Acquire(ctx, broker.QueueExpress))
	require.NoError(t, b.Acquire(ctx, broker.QueueStandard))

	acquired := make(chan struct{})
	go func() {
		_ = b.Acquire(ctx, broker.QueueExpress)
		close(acquired)
	}()

	select {
	case <-acquired:
		t.Fatal("expected bulkhead to block second express acquire")
	case <-time.After(50 * time.Millisecond):
	}

	b.Release(broker.QueueExpress)

	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("expected second acquire to succeed after release")
	}

	b.Release(broker.QueueExpress)
	b.Release(broker.QueueStandard)
}

func TestBulkheadExpressSeparateFromStandard(t *testing.T) {
	b := NewBulkhead(1, 2)
	ctx := context.Background()

	require.NoError(t, b.Acquire(ctx, broker.QueueExpress))
	require.NoError(t, b.Acquire(ctx, broker.QueueStandard))
	require.NoError(t, b.Acquire(ctx, broker.QueueStandard))

	acquired := make(chan struct{})
	go func() {
		_ = b.Acquire(ctx, broker.QueueExpress)
		close(acquired)
	}()

	select {
	case <-acquired:
		t.Fatal("express pool should be full while standard still has capacity")
	case <-time.After(50 * time.Millisecond):
	}

	b.Release(broker.QueueExpress)

	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("expected express acquire after release")
	}

	b.Release(broker.QueueExpress)
	b.Release(broker.QueueStandard)
	b.Release(broker.QueueStandard)
}
