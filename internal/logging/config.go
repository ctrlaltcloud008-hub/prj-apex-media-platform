package logging

import (
	"context"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel/trace"
)

type Logger struct {
	logger  *slog.Logger
	service string
	videoID string
	traceID string
	region  string
	spanID  string
}

func New(service, region, appEnv string) *Logger {

	var logger *slog.Logger

	if appEnv != "local" {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	} else {
		logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}

	return &Logger{
		logger:  logger,
		service: service,
		region:  region,
	}

}

func (l *Logger) WithVideoID(videoID string) *Logger {
	l.videoID = videoID
	return l
}

func (l *Logger) WithSpanContext(ctx context.Context) *Logger {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		l.traceID = ""
		l.spanID = ""
		return l
	}
	l.traceID = sc.TraceID().String()
	l.spanID = sc.SpanID().String()
	return l
}

func (l Logger) Info(ctx context.Context, eventType, msg string, attrs ...slog.Attr) {
	l.log(ctx, slog.LevelInfo, eventType, msg, attrs...)
}

func (l Logger) Error(ctx context.Context, eventType, msg string, attrs ...slog.Attr) {
	l.log(ctx, slog.LevelError, eventType, msg, attrs...)
}

func (l Logger) Debug(ctx context.Context, eventType, msg string, attrs ...slog.Attr) {
	l.log(ctx, slog.LevelDebug, eventType, msg, attrs...)
}

func (l Logger) Warn(ctx context.Context, eventType, msg string, attrs ...slog.Attr) {
	l.log(ctx, slog.LevelWarn, eventType, msg, attrs...)
}

func (l Logger) log(ctx context.Context, level slog.Level, eventType, msg string, attrs ...slog.Attr) {
	required := []slog.Attr{
		slog.String("video_id", l.videoID),
		slog.String("trace", l.traceID),
		slog.String("spanId", l.spanID),
		slog.String("service", l.service),
		slog.String("event_type", eventType),
		slog.String("region", l.region),
	}
	l.logger.LogAttrs(ctx, level, msg, append(required, attrs...)...)
}
