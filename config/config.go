package config

import (
	"errors"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

type Config struct {
	AppPort string
	DBDSN   string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	viper.AutomaticEnv()

	viper.SetDefault("APP_PORT", "8080")

	cfg := &Config{
		AppPort: viper.GetString("APP_PORT"),
		DBDSN:   viper.GetString("DB_DSN"),
	}

	if cfg.DBDSN == "" {
		return nil, errors.New("DB_DSN is required")
	}

	return cfg, nil
}
