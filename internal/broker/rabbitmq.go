package broker

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/shahriyar/arvan/internal/domain"
)

const (
	QueueExpress  = "sms.express"
	QueueStandard = "sms.standard"
)

type RabbitMQ struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	confirms chan amqp.Confirmation
	mu      sync.Mutex
}

func Connect(url string) (*RabbitMQ, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("dial rabbitmq: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("open rabbitmq channel: %w", err)
	}

	if err := ch.Confirm(false); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("enable publisher confirms: %w", err)
	}

	confirms := ch.NotifyPublish(make(chan amqp.Confirmation, 1))

	if err := declareTopology(ch); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, err
	}

	return &RabbitMQ{
		conn:     conn,
		channel:  ch,
		confirms: confirms,
	}, nil
}

func declareTopology(ch *amqp.Channel) error {
	queues := []string{QueueExpress, QueueStandard}
	for _, name := range queues {
		if _, err := ch.QueueDeclare(
			name,
			true,  // durable
			false, // auto-delete
			false, // exclusive
			false, // no-wait
			amqp.Table{"x-queue-type": "classic"},
		); err != nil {
			return fmt.Errorf("declare queue %s: %w", name, err)
		}
	}
	return nil
}

func QueueForMessageType(messageType string) string {
	if messageType == domain.MessageTypeExpress {
		return QueueExpress
	}
	return QueueStandard
}

func (r *RabbitMQ) Publish(ctx context.Context, queue string, body []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	err := r.channel.PublishWithContext(
		ctx,
		"",
		queue,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         body,
		},
	)
	if err != nil {
		return fmt.Errorf("publish to %s: %w", queue, err)
	}

	select {
	case confirm := <-r.confirms:
		if !confirm.Ack {
			return fmt.Errorf("publish to %s not acknowledged", queue)
		}
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

func (r *RabbitMQ) Consume(queue string, prefetch int) (<-chan amqp.Delivery, error) {
	if prefetch <= 0 {
		prefetch = 10
	}

	ch, err := r.conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("open consumer channel: %w", err)
	}

	if err := ch.Qos(prefetch, 0, false); err != nil {
		_ = ch.Close()
		return nil, fmt.Errorf("set qos: %w", err)
	}

	deliveries, err := ch.Consume(
		queue,
		"",
		false, // manual ack
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		_ = ch.Close()
		return nil, fmt.Errorf("consume %s: %w", queue, err)
	}

	return deliveries, nil
}

func (r *RabbitMQ) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.channel != nil {
		if err := r.channel.Close(); err != nil {
			slog.Warn("close rabbitmq publish channel", "error", err)
		}
	}
	if r.conn != nil {
		if err := r.conn.Close(); err != nil {
			return fmt.Errorf("close rabbitmq connection: %w", err)
		}
	}
	return nil
}
