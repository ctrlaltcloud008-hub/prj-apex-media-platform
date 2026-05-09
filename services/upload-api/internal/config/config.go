package config

import (
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/config"
	"github.com/spf13/viper"
)

type UploadConfig struct {
	appEnv    string
	port      string
	service   string
	region    string
	projectID string
	spannerDB string
	buckets   map[string]string
}

func LoadUploadConfig() (*UploadConfig, error) {

	v := viper.New()
	v.SetDefault("PORT", ":8080")
	v.SetDefault("APP_ENV", "local")
	v.SetDefault("SERVICE", "upload")
	v.SetDefault("REGION", "asia-south1")
	v.SetDefault("PROJECT_ID", "apex-494315")
	v.SetDefault("BUCKETS", map[string]string{})
	v.SetDefault("SPANNER_DATABASE", "")

	if err := config.LoadConfig(v, "upload"); err != nil {
		return nil, err
	}

	cfg := &UploadConfig{
		appEnv:    v.GetString("APP_ENV"),
		port:      v.GetString("PORT"),
		service:   v.GetString("SERVICE"),
		region:    v.GetString("REGION"),
		projectID: v.GetString("PROJECT_ID"),
		buckets:   v.GetStringMapString("BUCKETS"),
		spannerDB: v.GetString("SPANNER_DATABASE"),
	}

	return cfg, nil
}

func (c *UploadConfig) AppEnv() string    { return c.appEnv }
func (c *UploadConfig) Port() string      { return c.port }
func (c *UploadConfig) Service() string   { return c.service }
func (c *UploadConfig) Region() string    { return c.region }
func (c *UploadConfig) ProjectID() string { return c.projectID }
func (c *UploadConfig) Buckets() map[string]string {
	return c.buckets
}
func (c *UploadConfig) SpannerDatabase() string {
	return c.spannerDB
}
