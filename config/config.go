package config

import (
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
	viper.SetDefault("DB_DSN", "host=localhost user=postgres password=postgres dbname=journey_db port=5432 sslmode=disable")

	return &Config{
		AppPort: viper.GetString("APP_PORT"),
		DBDSN:   viper.GetString("DB_DSN"),
	}, nil
}
