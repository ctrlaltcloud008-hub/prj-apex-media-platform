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
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/ingestion/internal/config"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/ingestion/internal/gcs"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/ingestion/internal/handler"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("ingestion service failed: %v", err)
	}
}

func run() error {

	cfg, err := config.LoadIngestionConfig()
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

	storage, err := gcs.NewClient(ctx)
	if err != nil {
		return err
	}
	defer storage.Close()

	spannerClient, err := spanner.NewClient(ctx, cfg.SpannerDatabase(), spanner.DefaultConfig())
	if err != nil {
		return err
	}
	defer spannerClient.Close()

	messageHandler := handler.NewHandler(logger, subscriber, storage, spannerClient, cfg.Region())
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

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
		if err := messageHandler.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			handlerErrCh <- fmt.Errorf("message handler: %w", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		logger.Info(ctx, "server.starting", "Starting HTTP server", slog.String("addr", cfg.Port()))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrCh <- fmt.Errorf("http server: %w", err)
		}
	}()

	var runErr error
	select {
	case <-ctx.Done():
		logger.Info(ctx, "shutdown.initiated", "Shutdown signal received")
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
