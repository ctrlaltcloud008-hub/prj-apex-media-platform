package poller

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	gcppubsub "cloud.google.com/go/pubsub/v2"
	"cloud.google.com/go/spanner"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/logging"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/outbox"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/outbox-poller/internal/publisher"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/outbox-poller/internal/store"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const instrumentationName = "services/outbox-poller/internal/poller"

type Poller struct {
	spanner        *spanner.Client
	publisher      *publisher.TopicPublisher
	batchSize      int64
	assignedShards []int64
	logger         *logging.Logger
	tracer         trace.Tracer
}

type pendingPublish struct {
	entryID string
	videoID string
	result  *gcppubsub.PublishResult
	span    trace.Span
}

func NewPoller(
	spannerClient *spanner.Client,
	topicPublisher *publisher.TopicPublisher,
	batchSize int64,
	assignedShards []int64,
	logger *logging.Logger,
) *Poller {
	shards := append([]int64(nil), assignedShards...)

	return &Poller{
		spanner:        spannerClient,
		publisher:      topicPublisher,
		batchSize:      batchSize,
		assignedShards: shards,
		logger:         logger,
		tracer:         otel.Tracer(instrumentationName),
	}
}

func (p *Poller) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var (
		running atomic.Bool
		wg      sync.WaitGroup
	)
	defer wg.Wait()

	runTick := func() {
		if !running.CompareAndSwap(false, true) {
			recordTickSkipped(ctx)
			return
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			defer running.Store(false)

			if err := p.Poll(ctx); err != nil && ctx.Err() == nil {
				p.logger.Error(ctx, "outbox.poll_failed", "Outbox poll failed", slog.String("error", err.Error()))
			}
		}()
	}

	runTick()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runTick()
		}
	}
}

func (p *Poller) Poll(ctx context.Context) error {
	for _, shardID := range p.assignedShards {
		rows, err := store.ReadPendingOutboxEntries(ctx, p.spanner, shardID, p.batchSize)
		if err != nil {
			return fmt.Errorf("read pending outbox entries for shard %d: %w", shardID, err)
		}

		if len(rows) == 0 {
			continue
		}

		for topic, entries := range groupEntriesByTopic(rows) {
			if err := p.publishTopicGroup(ctx, shardID, topic, entries); err != nil {
				return err
			}
		}
	}

	return nil
}

func (p *Poller) publishTopicGroup(ctx context.Context, shardID int64, topic string, entries []store.PendingOutboxEntry) error {
	publishes := make([]pendingPublish, 0, len(entries))
	for _, entry := range entries {
		env, data, err := parsePayload(entry.Payload)
		if err != nil {
			return fmt.Errorf("parse payload for entry %s: %w", entry.EntryID, err)
		}

		publishCtx, span := p.startPublishSpan(ctx, shardID, topic, entry, env)
		result := p.publisher.PublishFromOutbox(publishCtx, topic, env, data, map[string]string{
			"video_id": entry.VideoID,
		})

		publishes = append(publishes, pendingPublish{
			entryID: entry.EntryID,
			videoID: entry.VideoID,
			result:  result,
			span:    span,
		})
	}

	publishedEntryIDs := make([]string, 0, len(publishes))
	for _, publish := range publishes {
		_, err := publish.result.Get(ctx)
		if err != nil {
			publish.span.RecordError(err)
			publish.span.SetStatus(otelcodes.Error, "publish failed")
			publish.span.End()

			p.logger.Error(ctx, "outbox.publish_failed", "Failed to publish outbox entry",
				slog.Int64("shard_id", shardID),
				slog.String("topic", topic),
				slog.String("entry_id", publish.entryID),
				slog.String("video_id", publish.videoID),
				slog.String("error", err.Error()),
			)
			continue
		}

		publish.span.SetStatus(otelcodes.Ok, "")
		publish.span.End()
		publishedEntryIDs = append(publishedEntryIDs, publish.entryID)
	}

	if len(publishedEntryIDs) == 0 {
		return nil
	}

	if err := store.MarkOutboxEntriesPublished(ctx, p.spanner, publishedEntryIDs); err != nil {
		return fmt.Errorf("mark published entries for shard %d topic %q: %w", shardID, topic, err)
	}

	p.logger.Info(ctx, "outbox.publish_success", "Published outbox batch",
		slog.Int64("shard_id", shardID),
		slog.String("topic", topic),
		slog.Int("count", len(publishedEntryIDs)),
	)

	return nil
}

func groupEntriesByTopic(entries []store.PendingOutboxEntry) map[string][]store.PendingOutboxEntry {
	grouped := make(map[string][]store.PendingOutboxEntry)
	for _, entry := range entries {
		grouped[entry.Topic] = append(grouped[entry.Topic], entry)
	}

	return grouped
}

func parsePayload(payload spanner.NullJSON) (outbox.Envelope, []byte, error) {
	if !payload.Valid {
		return outbox.Envelope{}, nil, fmt.Errorf("payload is null")
	}

	raw, err := json.Marshal(payload.Value)
	if err != nil {
		return outbox.Envelope{}, nil, fmt.Errorf("marshal JSON payload: %w", err)
	}

	env, err := outbox.ParseEnvelope(raw)
	if err != nil {
		return outbox.Envelope{}, nil, err
	}

	data, err := json.Marshal(env.Data)
	if err != nil {
		return outbox.Envelope{}, nil, fmt.Errorf("marshal envelope data: %w", err)
	}

	return env, data, nil
}

func (p *Poller) startPublishSpan(ctx context.Context, shardID int64, topic string, entry store.PendingOutboxEntry, env outbox.Envelope) (context.Context, trace.Span) {
	attrs := []attribute.KeyValue{
		attribute.String("messaging.system", "gcp_pubsub"),
		attribute.String("messaging.destination.name", topic),
		attribute.String("outbox.entry_id", entry.EntryID),
		attribute.Int64("outbox.shard_id", shardID),
		attribute.String("video.id", entry.VideoID),
	}

	opts := []trace.SpanStartOption{
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(attrs...),
	}

	if link, ok := envelopeLink(env); ok {
		opts = append(opts, trace.WithLinks(link))
	}

	return p.tracer.Start(ctx, "outbox.publish", opts...)
}

func envelopeLink(env outbox.Envelope) (trace.Link, bool) {
	carrier := propagation.MapCarrier{}
	if env.Traceparent != "" {
		carrier["traceparent"] = env.Traceparent
	}
	if env.Tracestate != "" {
		carrier["tracestate"] = env.Tracestate
	}
	if len(carrier) == 0 {
		return trace.Link{}, false
	}

	linkCtx := otel.GetTextMapPropagator().Extract(context.Background(), carrier)
	spanContext := trace.SpanContextFromContext(linkCtx)
	if !spanContext.IsValid() {
		return trace.Link{}, false
	}

	return trace.Link{SpanContext: spanContext}, true
}
