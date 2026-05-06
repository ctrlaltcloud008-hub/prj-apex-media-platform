package handler

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	metricapi "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const instrumentationName = "services/ingestion/internal/handler"

type ingestionTelemetry struct {
	tracer               trace.Tracer
	processedTotal       metricapi.Int64Counter
	processingDuration   metricapi.Float64Histogram
	ffprobeDuration      metricapi.Float64Histogram
	profileSelectedTotal metricapi.Int64Counter
	dedupHitTotal        metricapi.Int64Counter
	fileSizeBytes        metricapi.Int64Histogram
	validationFailures   metricapi.Int64Counter
}

var (
	telemetryOnce sync.Once
	telemetry     ingestionTelemetry
)

func getTelemetry() ingestionTelemetry {
	telemetryOnce.Do(func() {
		meter := otel.Meter(instrumentationName)
		telemetry = ingestionTelemetry{
			tracer:               otel.Tracer(instrumentationName),
			processedTotal:       mustInt64Counter(meter, "ingestion_processed_total", "Total ingestion message processing attempts by terminal status."),
			processingDuration:   mustFloat64Histogram(meter, "ingestion_duration_seconds", "s", "End-to-end ingestion processing duration in seconds."),
			ffprobeDuration:      mustFloat64Histogram(meter, "ingestion_ffprobe_duration_seconds", "s", "ffprobe extraction duration in seconds."),
			profileSelectedTotal: mustInt64Counter(meter, "ingestion_profile_selected_total", "Total selected transcode profiles."),
			dedupHitTotal:        mustInt64Counter(meter, "ingestion_dedup_hit_total", "Total deduplicated ingestion messages."),
			fileSizeBytes:        mustInt64Histogram(meter, "ingestion_file_size_bytes", "By", "Uploaded file sizes observed by ingestion."),
			validationFailures:   mustInt64Counter(meter, "ingestion_validation_failure_total", "Total ingestion validation failures by reason."),
		}
	})

	return telemetry
}

func mustInt64Counter(meter metricapi.Meter, name, description string) metricapi.Int64Counter {
	counter, err := meter.Int64Counter(name, metricapi.WithDescription(description))
	if err != nil {
		panic(err)
	}
	return counter
}

func mustInt64Histogram(meter metricapi.Meter, name, unit, description string) metricapi.Int64Histogram {
	histogram, err := meter.Int64Histogram(name, metricapi.WithUnit(unit), metricapi.WithDescription(description))
	if err != nil {
		panic(err)
	}
	return histogram
}

func mustFloat64Histogram(meter metricapi.Meter, name, unit, description string) metricapi.Float64Histogram {
	histogram, err := meter.Float64Histogram(name, metricapi.WithUnit(unit), metricapi.WithDescription(description))
	if err != nil {
		panic(err)
	}
	return histogram
}

func startStageSpan(ctx context.Context, spanName string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return getTelemetry().tracer.Start(ctx, spanName, trace.WithAttributes(attrs...))
}

func recordSpanError(span trace.Span, err error, description string) {
	if err == nil {
		return
	}

	span.RecordError(err)
	span.SetStatus(otelcodes.Error, description)
}

func recordProcessingMetrics(ctx context.Context, startedAt time.Time, region, status, failureReason string) {
	metric := getTelemetry()
	metric.processingDuration.Record(ctx, time.Since(startedAt).Seconds(), metricapi.WithAttributes(attribute.String("region", region)))
	if status != "" {
		metric.processedTotal.Add(ctx, 1, metricapi.WithAttributes(attribute.String("status", status)))
	}
	if failureReason != "" {
		metric.validationFailures.Add(ctx, 1, metricapi.WithAttributes(attribute.String("reason", failureReason)))
	}
	if status == "dedup" {
		metric.dedupHitTotal.Add(ctx, 1)
	}
}
