package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/gcs"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/logging"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/otel"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/spanner"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/upload-api/internal/config"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/upload-api/internal/handler"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/upload-api/internal/middleware"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/upload-api/internal/services"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("upload-api service failed: %v", err)
	}
}

func run() error {

	cfg, err := config.LoadUploadConfig()
	if err != nil {
		return err
	}

	logger := logging.New(cfg.Service(), cfg.Region(), cfg.AppEnv())
	bucketRegions := make([]string, 0, len(cfg.Buckets()))
	for region := range cfg.Buckets() {
		bucketRegions = append(bucketRegions, region)
	}
	sort.Strings(bucketRegions)

	logger.Info(
		context.Background(),
		"service.startup",
		"Upload API bootstrap started",
		slog.String("component", "bootstrap"),
		slog.String("app_env", cfg.AppEnv()),
		slog.String("http_addr", cfg.Port()),
		slog.String("project_id", cfg.ProjectID()),
		slog.String("spanner_database", cfg.SpannerDatabase()),
		slog.Int("bucket_region_count", len(bucketRegions)),
		slog.String("configured_bucket_regions", strings.Join(bucketRegions, ",")),
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

	gcsClient, err := gcs.NewClient(ctx)
	if err != nil {
		return err
	}
	defer gcsClient.Close()
	logger.Info(
		ctx,
		"gcs.client_initialized",
		"GCS client initialized",
		slog.String("component", "bootstrap"),
		slog.Int("bucket_region_count", len(bucketRegions)),
	)

	uploadService := services.NewUploadService(logger, cfg, spannerClient, gcsClient)
	logger.Info(
		ctx,
		"upload.service_initialized",
		"Upload service initialized",
		slog.String("component", "bootstrap"),
	)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handler.Healthz)
	mux.HandleFunc("POST /upload", handler.Upload(logger, uploadService))

	wrapped := middleware.Chain(
		mux,
		middleware.RequestID(logger),
		middleware.Authentication(logger),
		middleware.RequestLogging(logger),
	)

	server := &http.Server{
		Addr:    cfg.Port(),
		Handler: wrapped,
	}
	logger.Info(
		ctx,
		"server.configured",
		"HTTP server configured",
		slog.String("component", "http_server"),
		slog.String("addr", cfg.Port()),
		slog.String("routes", "GET /healthz,POST /upload"),
		slog.Bool("request_id_middleware", true),
		slog.Bool("authentication_middleware", true),
		slog.Bool("request_logging_middleware", true),
	)

	serverErrch := make(chan error, 1)
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
		if err := server.ListenAndServe(); err != nil {
			if err == http.ErrServerClosed {
				logger.Info(
					context.Background(),
					"server.stopped",
					"HTTP server stopped accepting new requests",
					slog.String("component", "http_server"),
					slog.String("addr", cfg.Port()),
				)
				return
			}
			serverErrch <- err
		}
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
	case err := <-serverErrch:
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

	shutdownTimeout := 10 * time.Second
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	logger.Info(
		context.Background(),
		"server.shutdown_started",
		"Starting HTTP server shutdown",
		slog.String("component", "http_server"),
		slog.String("addr", cfg.Port()),
		slog.Int("timeout_seconds", int(shutdownTimeout.Seconds())),
	)

	if err := server.Shutdown(shutdownCtx); err != nil && err != http.ErrServerClosed {
		logger.Error(
			context.Background(),
			"server.shutdown_failed",
			"Failed to shutdown HTTP server",
			slog.String("component", "http_server"),
			slog.String("addr", cfg.Port()),
			slog.String("error", err.Error()),
			slog.Int("timeout_seconds", int(shutdownTimeout.Seconds())),
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
			slog.Int("timeout_seconds", int(shutdownTimeout.Seconds())),
		)
	}

	wg.Wait()
	logger.Info(
		context.Background(),
		"service.stopped",
		"Upload API stopped",
		slog.String("component", "lifecycle"),
		slog.Bool("clean_shutdown", runErr == nil),
	)
	return runErr
}
