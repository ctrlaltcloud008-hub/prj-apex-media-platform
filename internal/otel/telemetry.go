package otel

import (
	"context"
	"errors"

	"go.opentelemetry.io/contrib/detectors/gcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/oauth"
)

type TracerConfig struct {
	AppEnv      string
	ServiceName string
	ProjectID   string
}

const TracerName = "internal/otel"

func InitTracer(ctx context.Context, cfg TracerConfig) (func(ctx context.Context) error, error) {

	var shutdownFuncs []func(ctx context.Context) error

	shutdown := func(ctx context.Context) error {
		var err error
		for i := len(shutdownFuncs) - 1; i >= 0; i-- {
			err = errors.Join(err, shutdownFuncs[i](ctx))
		}
		shutdownFuncs = nil
		return err
	}

	cleanupOnError := func(err error) error {
		return errors.Join(err, shutdown(ctx))
	}

	prop := propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})

	otel.SetTextMapPropagator(prop)

	res, err := sdkresource.New(
		ctx,
		sdkresource.WithTelemetrySDK(),
		sdkresource.WithDetectors(gcp.NewDetector()),
		sdkresource.WithAttributes(
			attribute.String("service.name", cfg.ServiceName),
			attribute.String("service.environment", cfg.AppEnv),
			attribute.String("project.id", cfg.ProjectID),
		),
	)

	if err != nil {
		return shutdown, cleanupOnError(err)
	}

	if cfg.AppEnv == "local" {
		tp := sdktrace.NewTracerProvider(
			sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample())),
			sdktrace.WithResource(res),
		)

		shutdownFuncs = append(shutdownFuncs, tp.Shutdown)
		otel.SetTracerProvider(tp)
		return shutdown, nil
	}

	creds, err := oauth.NewApplicationDefault(ctx)
	if err != nil {
		return shutdown, cleanupOnError(err)
	}

	exporter, err := otlptracegrpc.New(
		ctx,
		otlptracegrpc.WithDialOption(grpc.WithPerRPCCredentials(creds)),
		otlptracegrpc.WithHeaders(map[string]string{
			"x-goog-user-project": cfg.ProjectID,
		}))

	if err != nil {
		return shutdown, cleanupOnError(err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample())),
		sdktrace.WithBatcher(exporter), sdktrace.WithResource(res))

	shutdownFuncs = append(shutdownFuncs, tp.Shutdown)
	otel.SetTracerProvider(tp)

	return shutdown, nil
}
