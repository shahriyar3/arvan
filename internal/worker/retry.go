package worker

import amqp "github.com/rabbitmq/amqp091-go"

func deliveryAttemptCount(d amqp.Delivery) int {
	if total := xDeathCount(d.Headers); total > 0 {
		return total
	}
	if d.Redelivered {
		return 1
	}
	return 0
}

func xDeathCount(headers amqp.Table) int {
	if headers == nil {
		return 0
	}

	raw, ok := headers["x-death"]
	if !ok {
		return 0
	}

	total := 0
	for _, table := range deathRecords(raw) {
		total += headerInt(table["count"])
	}
	return total
}

func deathRecords(raw any) []amqp.Table {
	deaths, ok := raw.([]any)
	if !ok {
		return nil
	}
	return tablesFromSlice(deaths)
}

func tablesFromSlice(items []any) []amqp.Table {
	out := make([]amqp.Table, 0, len(items))
	for _, item := range items {
		if table, ok := item.(amqp.Table); ok {
			out = append(out, table)
		}
	}
	return out
}

func headerInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	case uint:
		return int(n)
	case uint32:
		return int(n)
	case uint64:
		return int(n)
	case float32:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}
