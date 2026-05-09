package poller

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel"
	metricapi "go.opentelemetry.io/otel/metric"
)

type pollerTelemetry struct {
	tickSkippedTotal metricapi.Int64Counter
}

var (
	telemetryOnce sync.Once
	telemetry     pollerTelemetry
)

func getTelemetry() pollerTelemetry {
	telemetryOnce.Do(func() {
		meter := otel.Meter(instrumentationName)
		telemetry = pollerTelemetry{
			tickSkippedTotal: mustInt64Counter(meter, "outbox_poller_tick_skipped_total", "Total poller ticks skipped because a previous tick was still running."),
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

func recordTickSkipped(ctx context.Context) {
	getTelemetry().tickSkippedTotal.Add(ctx, 1)
}
