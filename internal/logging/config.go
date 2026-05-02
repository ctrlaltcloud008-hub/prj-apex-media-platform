package logging

import (
	"log/slog"
	"os"

	gcplog "github.com/googleapis/slog-handler-go"
)

type Config struct {
	Service   string
	Region    string
	ProjectID string
	Level     slog.Level
}

func New(cfg Config) *slog.Logger {
	handler := gcplog.NewHandler(os.Stdout, *gcplog.Options{
		ProjectID: cfg.ProjectID,
		Level:     cfg.Level,
	})
}
