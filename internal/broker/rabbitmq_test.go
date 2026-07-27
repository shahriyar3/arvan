package broker

import (
	"testing"

	"github.com/shahriyar/arvan/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestQueueForMessageType(t *testing.T) {
	assert.Equal(t, QueueExpress, QueueForMessageType(domain.MessageTypeExpress))
	assert.Equal(t, QueueStandard, QueueForMessageType(domain.MessageTypeStandard))
	assert.Equal(t, QueueStandard, QueueForMessageType("unknown"))
}
