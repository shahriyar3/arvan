package worker

import (
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
)

func TestDeliveryAttemptCountFromRedelivered(t *testing.T) {
	assert.Equal(t, 1, deliveryAttemptCount(amqp.Delivery{Redelivered: true}))
	assert.Equal(t, 0, deliveryAttemptCount(amqp.Delivery{}))
}

func TestDeliveryAttemptCountFromXDeath(t *testing.T) {
	delivery := amqp.Delivery{
		Headers: amqp.Table{
			"x-death": []any{
				amqp.Table{"count": int64(3)},
			},
		},
	}
	assert.Equal(t, 3, deliveryAttemptCount(delivery))
}

func TestDeliveryAttemptCountFromXDeathUint64(t *testing.T) {
	delivery := amqp.Delivery{
		Headers: amqp.Table{
			"x-death": []any{
				amqp.Table{"count": uint64(5)},
			},
		},
	}
	assert.Equal(t, 5, deliveryAttemptCount(delivery))
}

func TestDeliveryAttemptCountSumsMultipleXDeathRecords(t *testing.T) {
	delivery := amqp.Delivery{
		Headers: amqp.Table{
			"x-death": []any{
				amqp.Table{"count": int64(2)},
				amqp.Table{"count": float64(3)},
			},
		},
	}
	assert.Equal(t, 5, deliveryAttemptCount(delivery))
}

func TestDeliveryAttemptCountPrefersXDeathOverRedelivered(t *testing.T) {
	delivery := amqp.Delivery{
		Redelivered: true,
		Headers: amqp.Table{
			"x-death": []any{
				amqp.Table{"count": int64(4)},
			},
		},
	}
	assert.Equal(t, 4, deliveryAttemptCount(delivery))
}

func TestHeaderInt(t *testing.T) {
	assert.Equal(t, 7, headerInt(int64(7)))
	assert.Equal(t, 7, headerInt(uint64(7)))
	assert.Equal(t, 0, headerInt("not-a-number"))
}
