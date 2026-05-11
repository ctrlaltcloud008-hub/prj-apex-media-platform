package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/logging"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/otel"
	pbclient "github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/pubsub"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/spanner"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/outbox-poller/internal/config"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/outbox-poller/internal/poller"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/outbox-poller/internal/publisher"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("outbox poller service failed: %v", err)
	}
}

func run() error {

	cfg, err := config.LoadOutboxPollerConfig()
	if err != nil {
		return err
	}

	logger := logging.New(cfg.Service(), cfg.Region(), cfg.AppEnv())
	logger.Info(
		context.Background(),
		"service.startup",
		"Outbox poller bootstrap started",
		slog.String("component", "bootstrap"),
		slog.String("app_env", cfg.AppEnv()),
		slog.String("http_addr", cfg.Port()),
		slog.String("project_id", cfg.ProjectID()),
		slog.String("spanner_database", cfg.SpannerDatabase()),
		slog.Int64("batch_size", cfg.BatchSize()),
		slog.Int("poll_interval_ms", cfg.PollIntervalMS()),
		slog.Int("shard_count", cfg.ShardCount()),
	)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	trcfg := otel.TracerConfig{
		AppEnv:      cfg.AppEnv(),
		ServiceName: cfg.Service(),
		ProjectID:   cfg.ProjectID(),
		Region:      cfg.Region(),
	}

	shutdown, err := otel.InitTracer(ctx, trcfg)
	if err != nil {
		return err
	}
	logger.Info(
		ctx,
		"telemetry.initialized",
		"OpenTelemetry initialized",
		slog.String("component", "telemetry"),
		slog.String("project_id", cfg.ProjectID()),
	)

	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdown(shutdownCtx); err != nil {
			logger.Error(
				context.Background(),
				"tracer.shutdown_failed",
				"Failed to shutdown tracer",
				slog.String("component", "telemetry"),
				slog.String("error", err.Error()),
				slog.Int("timeout_seconds", 5),
			)
			return
		}
		logger.Info(
			context.Background(),
			"tracer.shutdown_success",
			"Tracer shutdown completed",
			slog.String("component", "telemetry"),
			slog.Int("timeout_seconds", 5),
		)
	}()

	client, err := pbclient.NewClient(ctx, pbclient.Config{
		ProjectID:      cfg.ProjectID(),
		EnabledTracing: true,
	})

	if err != nil {
		return err
	}

	defer client.Close()
	logger.Info(
		ctx,
		"pubsub.client_initialized",
		"Pub/Sub client initialized",
		slog.String("component", "bootstrap"),
		slog.String("project_id", cfg.ProjectID()),
		slog.Bool("enabled_tracing", true),
	)

	publisher := publisher.NewTopicPublisher(client)
	defer publisher.Stop()

	spannerClient, err := spanner.NewClient(ctx, cfg.SpannerDatabase(), spanner.DefaultConfig())

	if err != nil {
		return err
	}

	defer spannerClient.Close()
	logger.Info(
		ctx,
		"spanner.client_initialized",
		"Spanner client initialized",
		slog.String("component", "bootstrap"),
		slog.String("spanner_database", cfg.SpannerDatabase()),
	)

	assignedShards := allShards(cfg.ShardCount())
	p := poller.NewPoller(spannerClient, publisher, cfg.BatchSize(), assignedShards, logger)
	logger.Info(
		ctx,
		"outbox_poller.initialized",
		"Outbox poller initialized",
		slog.String("component", "bootstrap"),
		slog.Int64("batch_size", cfg.BatchSize()),
		slog.Int("poll_interval_ms", cfg.PollIntervalMS()),
		slog.Int("assigned_shard_count", len(assignedShards)),
		slog.Any("assigned_shards", assignedShards),
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	server := &http.Server{
		Addr:    cfg.Port(),
		Handler: mux,
	}
	logger.Info(
		ctx,
		"server.configured",
		"HTTP server configured",
		slog.String("component", "http_server"),
		slog.String("addr", cfg.Port()),
		slog.String("routes", "GET /healthz"),
	)

	serverErrCh := make(chan error, 1)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		logger.Info(
			ctx,
			"server.starting",
			"Starting HTTP server",
			slog.String("component", "http_server"),
			slog.String("addr", cfg.Port()),
		)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrCh <- err
			return
		}
		logger.Info(
			context.Background(),
			"server.stopped",
			"HTTP server stopped accepting new requests",
			slog.String("component", "http_server"),
			slog.String("addr", cfg.Port()),
		)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		logger.Info(ctx, "outbox_poller.starting", "Starting outbox poller",
			slog.String("component", "poller"),
			slog.Int64("batch_size", cfg.BatchSize()),
			slog.Int("poll_interval_ms", cfg.PollIntervalMS()),
			slog.Any("assigned_shards", assignedShards),
		)

		p.Run(ctx, time.Duration(cfg.PollIntervalMS())*time.Millisecond)
	}()

	var runErr error
	select {
	case <-ctx.Done():
		logger.Info(
			ctx,
			"shutdown.initiated",
			"Shutdown signal received",
			slog.String("component", "lifecycle"),
			slog.String("reason", "signal"),
		)

	case err := <-serverErrCh:
		runErr = err
		logger.Error(
			ctx,
			"server.failed",
			"HTTP server failed",
			slog.String("component", "http_server"),
			slog.String("addr", cfg.Port()),
			slog.String("error", err.Error()),
		)
	}

	stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	logger.Info(
		context.Background(),
		"server.shutdown_started",
		"Starting HTTP server shutdown",
		slog.String("component", "http_server"),
		slog.String("addr", cfg.Port()),
		slog.Int("timeout_seconds", 10),
	)

	if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error(
			context.Background(),
			"server.shutdown_failed",
			"Failed to shutdown HTTP server",
			slog.String("component", "http_server"),
			slog.String("addr", cfg.Port()),
			slog.String("error", err.Error()),
			slog.Int("timeout_seconds", 10),
		)
		if runErr == nil {
			runErr = fmt.Errorf("shutdown http server: %w", err)
		}
	} else {
		logger.Info(
			context.Background(),
			"server.shutdown_success",
			"HTTP server shutdown gracefully",
			slog.String("component", "http_server"),
			slog.String("addr", cfg.Port()),
			slog.Int("timeout_seconds", 10),
		)
	}

	wg.Wait()
	logger.Info(
		context.Background(),
		"service.stopped",
		"Outbox poller stopped",
		slog.String("component", "lifecycle"),
		slog.Bool("clean_shutdown", runErr == nil),
	)

	return runErr
}

func allShards(n int) []int64 {
	shards := make([]int64, 0, n)
	for i := 0; i < n; i++ {
		shards = append(shards, int64(i))
	}

	return shards
}
