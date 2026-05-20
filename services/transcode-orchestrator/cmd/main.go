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
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/spanner"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/transcode-orchestrator/internal/config"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/transcode-orchestrator/internal/handler"

	pbclient "github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/pubsub"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("transcode orchestrator service failed: %v", err)
	}
}

func run() error {

	cfg, err := config.LoadTranscodeOrchestratorConfig()
	if err != nil {
		return err
	}

	logger := logging.New(cfg.Service(), cfg.Region(), cfg.AppEnv())

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	trCfg := otel.TracerConfig{
		AppEnv:      cfg.AppEnv(),
		ServiceName: cfg.Service(),
		ProjectID:   cfg.ProjectID(),
		Region:      cfg.Region(),
	}

	shutdown, err := otel.InitTracer(ctx, trCfg)
	if err != nil {
		return err
	}

	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdown(shutdownCtx); err != nil {
			logger.Error(ctx, "tracer.shutdown_failed", "Failed to shutdown tracer")
		}
	}()

	client, err := pbclient.NewClient(ctx, pbclient.Config{
		ProjectID:      cfg.ProjectID(),
		EnabledTracing: true,
	})

	if err != nil {
		return err
	}

	defer client.Close()

	subscriber := pbclient.NewSubscriber(client, cfg.Subscription(),
		pbclient.WithMaxOutstandingMessages(100),
		pbclient.WithNumGoroutines(10),
		pbclient.WithMaxOutstandingBytes(100*1024*1024),
	)

	spannerClient, err := spanner.NewClient(ctx, cfg.SpannerDB(), spanner.DefaultConfig())
	if err != nil {
		return err
	}
	defer spannerClient.Close()

	orchestrator := handler.NewHandler(
		logger,
		subscriber,
		spannerClient,
		cfg.Region(),
	)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handler.Healthz)

	server := &http.Server{
		Addr:    cfg.Port(),
		Handler: mux,
	}

	handlerErrCh := make(chan error, 1)
	serverErrCh := make(chan error, 1)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := orchestrator.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			handlerErrCh <- fmt.Errorf("message handler: %w", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		logger.Info(ctx, "http.server_starting", "Starting HTTP server", slog.String("port", cfg.Port()))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrCh <- fmt.Errorf("http server: %w", err)
		}
	}()

	var runErr error
	select {
	case <-ctx.Done():
		logger.Info(context.Background(), "shutdown.initiated", "Shutdown signal received, initiating graceful shutdown")
	case err := <-handlerErrCh:
		runErr = err
		logger.Error(ctx, "handler.failed", "Message handler stopped unexpectedly", slog.String("error", err.Error()))
	case err := <-serverErrCh:
		runErr = err
		logger.Error(ctx, "server.failed", "HTTP server failed", slog.String("error", err.Error()))
	}

	stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error(ctx, "server.shutdown_failed", "Failed to shutdown HTTP server", slog.String("error", err.Error()))
		if runErr == nil {
			runErr = fmt.Errorf("shutdown http server: %w", err)
		}
	} else {
		logger.Info(ctx, "server.shutdown_success", "HTTP server shutdown gracefully")
	}

	wg.Wait()

	return runErr
}
