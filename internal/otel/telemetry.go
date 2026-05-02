package otel

import (
	"context"
	"errors"

	"cloud.google.com/go/pubsub/v2"
	"go.opentelemetry.io/contrib/detectors/gcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/oauth"
)

type tracerConfig struct {
	AppEnv      string
	ServiceName string
	ProjectID   string
}

const TracerName = "internal/otel"

func InitTracer(ctx context.Context, cfg tracerConfig) (func(ctx context.Context) error, error) {

	var shutdownFuncs []func(ctx context.Context) error
	var err error

	shutdown := func(ctx context.Context) error {
		var err error
		for _, f := range shutdownFuncs {
			err = errors.Join(err, f(ctx))
		}
		shutdownFuncs = nil
		return err
	}

	handleError := func(err error) {
		err = errors.Join(err, shutdown(ctx))
	}

	prop := propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})

	otel.SetTextMapPropagator(prop)

	res, err := sdkresource.New(
		context.Background(),
		sdkresource.WithTelemetrySDK(),
		sdkresource.WithDetectors(gcp.NewDetector()),
		sdkresource.WithAttributes(
			attribute.String("service.name", cfg.ServiceName),
			attribute.String("service.environment", cfg.AppEnv),
			attribute.String("project.id", cfg.ProjectID),
		),
	)

	if err != nil {
		handleError(err)
		return shutdown, err
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
		handleError(err)
		return shutdown, err
	}

	exporter, err := otlptracegrpc.New(
		ctx,
		otlptracegrpc.WithDialOption(grpc.WithPerRPCCredentials(creds)),
		otlptracegrpc.WithHeaders(map[string]string{
			"x-goog-user-project": cfg.ProjectID,
		}))

	if err != nil {
		handleError(err)
		return shutdown, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample())),
		sdktrace.WithBatcher(exporter), sdktrace.WithResource(res))

	shutdownFuncs = append(shutdownFuncs, tp.Shutdown)
	otel.SetTracerProvider(tp)

	return shutdown, nil
}

func SpanFromPubSubMessage(ctx context.Context, msg *pubsub.Message, spanName string) (context.Context, trace.Span) {
	attrs := msg.Attributes
	if attrs == nil {
		attrs = map[string]string{}
	}
	ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier(attrs))
	ctx, span := otel.Tracer(TracerName).Start(
		ctx,
		spanName,
		trace.WithSpanKind(trace.SpanKindConsumer),
	)
	if videoID := attrs["video.id"]; videoID != "" {
		span.SetAttributes(attribute.String("video.id", videoID))
	} else if videoID := attrs["video_id"]; videoID != "" {
		span.SetAttributes(attribute.String("video.id", videoID))
	}
	return ctx, span
}
func InjectIntoPubSubMessage(ctx context.Context, msg *pubsub.Message) {
	if msg.Attributes == nil {
		msg.Attributes = make(map[string]string)
	}
	otel.GetTextMapPropagator().Inject(ctx, propagation.MapCarrier(msg.Attributes))
}
