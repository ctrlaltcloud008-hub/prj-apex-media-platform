package config

import (
	"errors"
	"fmt"

	"github.com/spf13/viper"
)

type IngestionCfg struct {
	appEnv string
}

func LoadIngestionConfig() (*IngestionCfg, error) {

	v := viper.New()

	if err := loadConfig(v, "ingestion"); err != nil {
		return nil, err
	}

	v.SetDefault("PORT", ":8080")
	v.SetDefault("APP_ENV", "local")

	cfg := &IngestionCfg{
		appEnv: v.GetString("APP_ENV"),
	}

	return cfg, nil
}

func loadConfig(v *viper.Viper, name string) error {
	v.SetConfigName(name)
	v.SetConfigType("yaml")
	v.AddConfigPath(".")

	if err := v.ReadInConfig(); err != nil {
		var notFoundErr viper.ConfigFileNotFoundError
		if !errors.As(err, &notFoundErr) {
			return fmt.Errorf("read config file(%s) : %w", name, err)
		}

	}

	v.AutomaticEnv()

	return nil
}

func (c *IngestionCfg) AppEnv() string { return c.appEnv }
