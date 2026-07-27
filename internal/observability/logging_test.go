package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestTraceContextHandlerAddsTraceID(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	var buf bytes.Buffer
	base := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(&traceContextHandler{next: base})

	ctx, span := Tracer().Start(context.Background(), "test")
	defer span.End()

	logger.InfoContext(ctx, "processing message", "message_id", "abc")

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	require.Equal(t, span.SpanContext().TraceID().String(), entry["trace_id"])
	require.Equal(t, "processing message", entry["msg"])
}

func TestTraceContextHandlerOmitsTraceIDWithoutSpan(t *testing.T) {
	var buf bytes.Buffer
	base := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(&traceContextHandler{next: base})

	logger.InfoContext(context.Background(), "startup")

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	_, hasTraceID := entry["trace_id"]
	require.False(t, hasTraceID)
}
