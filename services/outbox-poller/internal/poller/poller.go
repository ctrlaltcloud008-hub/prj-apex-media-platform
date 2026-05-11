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

	logger := p.logger.WithSpanContext(ctx)
	logger.Info(
		ctx,
		"outbox.poller_loop_started",
		"Started outbox poll loop",
		slog.Duration("interval", interval),
		slog.Int64("batch_size", p.batchSize),
		slog.Int("assigned_shard_count", len(p.assignedShards)),
		slog.Any("assigned_shards", p.assignedShards),
	)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var (
		running atomic.Bool
		wg      sync.WaitGroup
	)
	defer wg.Wait()

	runTick := func() {
		if !running.CompareAndSwap(false, true) {
			logger.Warn(
				ctx,
				"outbox.tick_skipped",
				"Skipped outbox poll tick because the previous tick is still running",
				slog.Int("assigned_shard_count", len(p.assignedShards)),
			)
			recordTickSkipped(ctx)
			return
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			defer running.Store(false)

			if err := p.Poll(ctx); err != nil && ctx.Err() == nil {
				logger.Error(ctx, "outbox.poll_failed", "Outbox poll failed", slog.String("error", err.Error()))
			}
		}()
	}

	runTick()

	for {
		select {
		case <-ctx.Done():
			logger.Info(ctx, "outbox.poller_loop_stopped", "Stopped outbox poll loop", slog.String("reason", "context_canceled"))
			return
		case <-ticker.C:
			runTick()
		}
	}
}

func (p *Poller) Poll(ctx context.Context) error {
	pollStart := time.Now()
	ctx, span := p.tracer.Start(ctx, "outbox.poll",
		trace.WithAttributes(
			attribute.Int64("outbox.batch_size", p.batchSize),
			attribute.Int("outbox.assigned_shard_count", len(p.assignedShards)),
		),
	)
	defer span.End()

	logger := p.logger.WithSpanContext(ctx)
	logger.Info(
		ctx,
		"outbox.poll_started",
		"Started outbox poll cycle",
		slog.Int64("batch_size", p.batchSize),
		slog.Int("assigned_shard_count", len(p.assignedShards)),
		slog.Any("assigned_shards", p.assignedShards),
	)

	totalEntries := 0
	totalPublished := 0
	totalTopics := 0

	defer func() {
		span.SetAttributes(
			attribute.Int("outbox.entries_read", totalEntries),
			attribute.Int("outbox.entries_published", totalPublished),
			attribute.Int("outbox.topic_group_count", totalTopics),
		)
		logger.Info(
			ctx,
			"outbox.poll_completed",
			"Completed outbox poll cycle",
			slog.Duration("duration", time.Since(pollStart)),
			slog.Int("entries_read", totalEntries),
			slog.Int("entries_published", totalPublished),
			slog.Int("topic_group_count", totalTopics),
		)
	}()

	for _, shardID := range p.assignedShards {
		shardCtx, shardSpan := p.tracer.Start(ctx, "outbox.poll.shard",
			trace.WithAttributes(attribute.Int64("outbox.shard_id", shardID)),
		)
		shardStart := time.Now()
		rows, err := store.ReadPendingOutboxEntries(ctx, p.spanner, shardID, p.batchSize)
		if err != nil {
			shardSpan.RecordError(err)
			shardSpan.SetStatus(otelcodes.Error, "read pending entries failed")
			shardSpan.End()
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "read pending entries failed")
			logger.Error(
				ctx,
				"outbox.shard_read_failed",
				"Failed to read pending outbox entries for shard",
				slog.Int64("shard_id", shardID),
				slog.String("error", err.Error()),
			)
			return fmt.Errorf("read pending outbox entries for shard %d: %w", shardID, err)
		}
		totalEntries += len(rows)
		shardSpan.SetAttributes(attribute.Int("outbox.entries_read", len(rows)))

		if len(rows) == 0 {
			shardSpan.AddEvent("outbox.shard.empty")
			shardSpan.SetStatus(otelcodes.Ok, "no pending entries")
			logger.Debug(
				shardCtx,
				"outbox.shard_empty",
				"No pending outbox entries for shard",
				slog.Int64("shard_id", shardID),
			)
			shardSpan.End()
			continue
		}

		grouped := groupEntriesByTopic(rows)
		totalTopics += len(grouped)
		shardLogger := logger.WithSpanContext(shardCtx)
		shardLogger.Info(
			shardCtx,
			"outbox.shard_loaded",
			"Loaded pending outbox entries for shard",
			slog.Int64("shard_id", shardID),
			slog.Int("entry_count", len(rows)),
			slog.Int("topic_group_count", len(grouped)),
		)

		shardPublished := 0
		for topic, entries := range grouped {
			publishedCount, err := p.publishTopicGroup(shardCtx, shardID, topic, entries)
			if err != nil {
				shardSpan.RecordError(err)
				shardSpan.SetStatus(otelcodes.Error, "publish topic group failed")
				span.RecordError(err)
				span.SetStatus(otelcodes.Error, "publish topic group failed")
				shardSpan.End()
				return err
			}
			shardPublished += publishedCount
			totalPublished += publishedCount
		}

		shardSpan.SetAttributes(attribute.Int("outbox.entries_published", shardPublished))
		shardSpan.SetStatus(otelcodes.Ok, "shard processed")
		shardLogger.Info(
			shardCtx,
			"outbox.shard_completed",
			"Completed outbox shard poll",
			slog.Int64("shard_id", shardID),
			slog.Duration("duration", time.Since(shardStart)),
			slog.Int("entry_count", len(rows)),
			slog.Int("published_count", shardPublished),
		)
		shardSpan.End()
	}

	span.SetStatus(otelcodes.Ok, "poll cycle completed")
	return nil
}

func (p *Poller) publishTopicGroup(ctx context.Context, shardID int64, topic string, entries []store.PendingOutboxEntry) (int, error) {
	groupCtx, groupSpan := p.tracer.Start(ctx, "outbox.publish.topic-group",
		trace.WithAttributes(
			attribute.String("messaging.system", "gcp_pubsub"),
			attribute.String("messaging.destination.name", topic),
			attribute.Int64("outbox.shard_id", shardID),
			attribute.Int("outbox.entry_count", len(entries)),
		),
	)
	defer groupSpan.End()

	logger := p.logger.WithSpanContext(groupCtx)
	logger.Info(
		groupCtx,
		"outbox.publish_started",
		"Started publishing outbox topic batch",
		slog.Int64("shard_id", shardID),
		slog.String("topic", topic),
		slog.Int("entry_count", len(entries)),
	)

	publishes := make([]pendingPublish, 0, len(entries))
	for _, entry := range entries {
		env, data, err := parsePayload(entry.Payload)
		if err != nil {
			groupSpan.RecordError(err)
			groupSpan.SetStatus(otelcodes.Error, "parse payload failed")
			logger.Error(
				groupCtx,
				"outbox.payload_parse_failed",
				"Failed to parse outbox payload",
				slog.Int64("shard_id", shardID),
				slog.String("topic", topic),
				slog.String("entry_id", entry.EntryID),
				slog.String("video_id", entry.VideoID),
				slog.String("error", err.Error()),
			)
			return 0, fmt.Errorf("parse payload for entry %s: %w", entry.EntryID, err)
		}

		publishCtx, span := p.startPublishSpan(groupCtx, shardID, topic, entry, env)
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
		_, err := publish.result.Get(groupCtx)
		if err != nil {
			publish.span.RecordError(err)
			publish.span.SetStatus(otelcodes.Error, "publish failed")
			publish.span.End()
			groupSpan.AddEvent("outbox.publish.entry_failed", trace.WithAttributes(
				attribute.String("outbox.entry_id", publish.entryID),
				attribute.String("video.id", publish.videoID),
			))

			logger.Error(groupCtx, "outbox.publish_failed", "Failed to publish outbox entry",
				slog.Int64("shard_id", shardID),
				slog.String("topic", topic),
				slog.String("entry_id", publish.entryID),
				slog.String("video_id", publish.videoID),
				slog.String("error", err.Error()),
			)
			continue
		}

		publish.span.SetStatus(otelcodes.Ok, "")
		publish.span.AddEvent("outbox.publish.entry_succeeded")
		publish.span.End()
		publishedEntryIDs = append(publishedEntryIDs, publish.entryID)
	}

	if len(publishedEntryIDs) == 0 {
		groupSpan.SetAttributes(attribute.Int("outbox.entries_published", 0))
		groupSpan.SetStatus(otelcodes.Ok, "no entries published")
		logger.Warn(
			groupCtx,
			"outbox.publish_no_successes",
			"Finished outbox topic batch without successful publishes",
			slog.Int64("shard_id", shardID),
			slog.String("topic", topic),
			slog.Int("entry_count", len(entries)),
		)
		return 0, nil
	}

	if err := store.MarkOutboxEntriesPublished(groupCtx, p.spanner, publishedEntryIDs); err != nil {
		groupSpan.RecordError(err)
		groupSpan.SetStatus(otelcodes.Error, "mark published entries failed")
		logger.Error(
			groupCtx,
			"outbox.mark_published_failed",
			"Failed to mark outbox entries as published",
			slog.Int64("shard_id", shardID),
			slog.String("topic", topic),
			slog.Int("count", len(publishedEntryIDs)),
			slog.String("error", err.Error()),
		)
		return 0, fmt.Errorf("mark published entries for shard %d topic %q: %w", shardID, topic, err)
	}

	groupSpan.SetAttributes(attribute.Int("outbox.entries_published", len(publishedEntryIDs)))
	groupSpan.SetStatus(otelcodes.Ok, "topic batch published")
	logger.Info(groupCtx, "outbox.publish_success", "Published outbox batch",
		slog.Int64("shard_id", shardID),
		slog.String("topic", topic),
		slog.Int("count", len(publishedEntryIDs)),
	)

	return len(publishedEntryIDs), nil
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
