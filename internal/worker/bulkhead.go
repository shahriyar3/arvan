package worker

import (
	"context"

	"github.com/shahriyar/arvan/internal/broker"
)

type Bulkhead struct {
	express  chan struct{}
	standard chan struct{}
}

func NewBulkhead(expressSize, standardSize int) *Bulkhead {
	if expressSize <= 0 {
		expressSize = 20
	}
	if standardSize <= 0 {
		standardSize = 50
	}
	return &Bulkhead{
		express:  make(chan struct{}, expressSize),
		standard: make(chan struct{}, standardSize),
	}
}

func (b *Bulkhead) Acquire(ctx context.Context, queue string) error {
	sem := b.standard
	if queue == broker.QueueExpress {
		sem = b.express
	}

	select {
	case sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *Bulkhead) Release(queue string) {
	sem := b.standard
	if queue == broker.QueueExpress {
		sem = b.express
	}

	select {
	case <-sem:
	default:
	}
}
