package config

import (
	"strings"

	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/config"
	"github.com/spf13/viper"
)

type TranscodeOrchestratorConfig struct {
	appEnv       string
	service      string
	port         string
	region       string
	projectID    string
	spannerDB    string
	subscription string
}

func LoadTranscodeOrchestratorConfig() (*TranscodeOrchestratorConfig, error) {
	v := viper.New()
	v.SetDefault("PORT", "8080")
	v.SetDefault("APP_ENV", "local")
	v.SetDefault("SERVICE", "transcode-orchestrator")
	v.SetDefault("REGION", "asia-south1")
	v.SetDefault("PROJECT_ID", "apex-494315")
	v.SetDefault("SPANNER_DATABASE", "")
	v.SetDefault("SUBSCRIPTION", "")

	if err := config.LoadConfig(v, "transcode-orchestrator-service"); err != nil {
		return nil, err
	}

	cfg := &TranscodeOrchestratorConfig{
		appEnv:       v.GetString("APP_ENV"),
		port:         normalizePort(v.GetString("PORT")),
		service:      v.GetString("SERVICE"),
		region:       v.GetString("REGION"),
		projectID:    v.GetString("PROJECT_ID"),
		spannerDB:    v.GetString("SPANNER_DATABASE"),
		subscription: v.GetString("SUBSCRIPTION"),
	}

	return cfg, nil
}

func normalizePort(port string) string {
	port = strings.TrimSpace(port)
	if port == "" {
		return ":8080"
	}
	if strings.HasPrefix(port, ":") {
		return port
	}
	return ":" + port
}

func (c *TranscodeOrchestratorConfig) AppEnv() string    { return c.appEnv }
func (c *TranscodeOrchestratorConfig) Port() string      { return c.port }
func (c *TranscodeOrchestratorConfig) Service() string   { return c.service }
func (c *TranscodeOrchestratorConfig) Region() string    { return c.region }
func (c *TranscodeOrchestratorConfig) ProjectID() string { return c.projectID }
func (c *TranscodeOrchestratorConfig) SpannerDB() string { return c.spannerDB }
func (c *TranscodeOrchestratorConfig) Subscription() string {
	return c.subscription
}
