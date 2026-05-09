package handler

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const instrumentationName = "services/upload-api/internal/handler"

func startSpan(ctx context.Context, spanName string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return otel.Tracer(instrumentationName).Start(ctx, spanName, trace.WithAttributes(attrs...))
}

func recordSpanError(span trace.Span, err error, description string) {
	if err == nil {
		return
	}

	span.RecordError(err)
	span.SetStatus(otelcodes.Error, description)
}
