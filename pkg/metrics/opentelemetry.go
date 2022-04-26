package metrics

import (
	"context"

	"github.com/traefik/traefik/v2/pkg/types"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric"
	"go.opentelemetry.io/otel/metric/nonrecording"
)

type instruments struct {
	reqCounter metric.Int64Counter
	reqGauge   metric.Int64UpDownCounter
}

// RegisterOpenTelemetry registers all OpenTelemetry metrics.
func RegisterOpenTelemetry(ctx context.Context, config *types.OpenTelemetry) Registry {
	var exporter *otlpmetric.Exporter
	meter := nonrecording.NewNoopMeterProvider().Meter("Traefik")

	// This is just a sample of memory stats to record from the Memstats
	heapAlloc, _ := meter.AsyncInt64().UpDownCounter("heapAllocs")
	gcCount, _ := meter.AsyncInt64().Counter("gcCount")
	gcPause, _ := meter.SyncFloat64().Histogram("gcPause")

	otel.Get

	return nil
}
