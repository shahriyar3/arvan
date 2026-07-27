package observability

import (
	"context"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func TestTraceContextRoundTripMap(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	ctx, span := Tracer().Start(context.Background(), "test-span")
	defer span.End()

	injected := InjectMap(ctx)
	require.NotEmpty(t, injected)

	restored := ExtractMap(context.Background(), injected)
	require.Equal(t, span.SpanContext().TraceID(), trace.SpanFromContext(restored).SpanContext().TraceID())
}

func TestTraceContextRoundTripAMQP(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	ctx, span := Tracer().Start(context.Background(), "publish")
	defer span.End()

	headers := InjectAMQP(ctx, amqp.Table{})
	require.NotEmpty(t, headers)

	restored := ExtractAMQP(context.Background(), headers)
	require.Equal(t, span.SpanContext().TraceID(), trace.SpanFromContext(restored).SpanContext().TraceID())
}

func TestExtractPrefersAMQPOverPayload(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	apiCtx, apiSpan := Tracer().Start(context.Background(), "sms.send")
	payload := InjectMap(apiCtx)
	apiSpan.End()

	relayCtx := ExtractMap(context.Background(), payload)
	relayCtx, relaySpan := Tracer().Start(relayCtx, "outbox.publish")
	headers := InjectAMQP(relayCtx, amqp.Table{})
	relaySpan.End()

	workerCtx := ExtractMap(context.Background(), payload)
	workerCtx = ExtractAMQP(workerCtx, headers)
	require.Equal(t, relaySpan.SpanContext().SpanID(), trace.SpanFromContext(workerCtx).SpanContext().SpanID())

	wrongCtx := ExtractAMQP(context.Background(), headers)
	wrongCtx = ExtractMap(wrongCtx, payload)
	require.Equal(t, apiSpan.SpanContext().SpanID(), trace.SpanFromContext(wrongCtx).SpanContext().SpanID())
}
