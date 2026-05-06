package config

import (
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/config"
	"github.com/spf13/viper"
)

type IngestionConfig struct {
	appEnv       string
	port         string
	service      string
	region       string
	projectID    string
	spannerDB    string
	subscription string
}

func LoadIngestionConfig() (*IngestionConfig, error) {

	v := viper.New()
	v.SetDefault("PORT", ":8080")
	v.SetDefault("APP_ENV", "local")
	v.SetDefault("SERVICE", "ingestion")
	v.SetDefault("REGION", "asia-south1")
	v.SetDefault("PROJECT_ID", "apex-494315")
	v.SetDefault("SPANNER_DATABASE", "")

	if err := config.LoadConfig(v, "ingestion"); err != nil {
		return nil, err
	}

	cfg := &IngestionConfig{
		appEnv:       v.GetString("APP_ENV"),
		port:         v.GetString("PORT"),
		service:      v.GetString("SERVICE"),
		region:       v.GetString("REGION"),
		projectID:    v.GetString("PROJECT_ID"),
		spannerDB:    v.GetString("SPANNER_DATABASE"),
		subscription: v.GetString("SUBSCRIPTION"),
	}

	return cfg, nil
}

func (c *IngestionConfig) AppEnv() string    { return c.appEnv }
func (c *IngestionConfig) Port() string      { return c.port }
func (c *IngestionConfig) Service() string   { return c.service }
func (c *IngestionConfig) Region() string    { return c.region }
func (c *IngestionConfig) ProjectID() string { return c.projectID }
func (c *IngestionConfig) SpannerDatabase() string {
	return c.spannerDB
}
func (c *IngestionConfig) Subscription() string { return c.subscription }
