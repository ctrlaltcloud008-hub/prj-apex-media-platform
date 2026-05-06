package pubsub

import (
	"context"
	"fmt"

	"cloud.google.com/go/pubsub/v2"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/outbox"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/api/option"
)

type Config struct {
	ProjectID      string
	EnabledTracing bool
	Endpoint       string
}

func NewClient(ctx context.Context, cfg Config) (*pubsub.Client, error) {
	if cfg.ProjectID == "" {
		return nil, fmt.Errorf("project ID is required")
	}

	clientCfg := pubsub.ClientConfig{
		EnableOpenTelemetryTracing: cfg.EnabledTracing,
	}

	var opts []option.ClientOption
	if cfg.Endpoint != "" {
		opts = append(opts, option.WithEndpoint(cfg.Endpoint))
	}

	client, err := pubsub.NewClientWithConfig(ctx, cfg.ProjectID, &clientCfg, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create pubsub client(%s): %w", cfg.ProjectID, err)
	}

	return client, nil
}

type Publisher struct {
	inner *pubsub.Publisher
	topic string
}

func NewPublisher(client *pubsub.Client, topicID string) *Publisher {
	return &Publisher{
		inner: client.Publisher(topicID),
		topic: topicID,
	}
}

func (p *Publisher) Publish(ctx context.Context, msg *pubsub.Message) *pubsub.PublishResult {
	if msg.Attributes == nil {
		msg.Attributes = make(map[string]string)
	}

	otel.GetTextMapPropagator().Inject(ctx, propagation.MapCarrier(msg.Attributes))
	return p.inner.Publish(ctx, msg)
}

func (p *Publisher) PublishFromOutbox(ctx context.Context, env outbox.Envelope, msg *pubsub.Message) *pubsub.PublishResult {
	if msg.Attributes == nil {
		msg.Attributes = make(map[string]string)
	}

	if env.Traceparent != "" {
		msg.Attributes["traceparent"] = env.Traceparent
	}
	if env.Tracestate != "" {
		msg.Attributes["tracestate"] = env.Tracestate
	}

	return p.inner.Publish(ctx, msg)
}

func (p *Publisher) Stop() { p.inner.Stop() }

func (p *Publisher) Topic() string { return p.topic }

type SubscriberOption func(*pubsub.Subscriber)

func WithMaxOutstandingMessages(max int) SubscriberOption {
	return func(s *pubsub.Subscriber) { s.ReceiveSettings.MaxOutstandingMessages = max }
}

func WithNumGoroutines(num int) SubscriberOption {
	return func(s *pubsub.Subscriber) { s.ReceiveSettings.NumGoroutines = num }
}

func WithMaxOutstandingBytes(max int) SubscriberOption {
	return func(s *pubsub.Subscriber) { s.ReceiveSettings.MaxOutstandingBytes = max }
}

type Subscriber struct {
	inner *pubsub.Subscriber
	sub   string
}

func NewSubscriber(client *pubsub.Client, subscriptionID string, opts ...SubscriberOption) *Subscriber {
	sub := client.Subscriber(subscriptionID)
	for _, opt := range opts {
		opt(sub)
	}
	return &Subscriber{inner: sub, sub: subscriptionID}
}

func (s *Subscriber) Receive(ctx context.Context, f func(ctx context.Context, msg *pubsub.Message)) error {
	return s.inner.Receive(ctx, f)
}

func (s *Subscriber) Subscription() string {
	return s.sub
}

func StartConsumerSpan(ctx context.Context, msg *pubsub.Message, spanName string) (context.Context, trace.Span) {
	attrs := msg.Attributes
	if attrs == nil {
		attrs = map[string]string{}
	}

	ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier(attrs))
	ctx, span := otel.Tracer("internal/pubsub").Start(
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
