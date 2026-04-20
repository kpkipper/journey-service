package config

import (
	"log"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	App   AppConfig
	DBDSN Postgres
}

type AppConfig struct {
	Port   int    `envconfig:"APP_PORT"`
	Name   string `envconfig:"APP_NAME"`
	ENV    string `envconfig:"APP_ENV"`
	Prefix string `envconfig:"APP_PREFIX"`
	APIKey string `envconfig:"APP_API_KEY"`
}

type Postgres struct {
	Host     string `envconfig:"POSTGRES_HOST"`
	Port     string `envconfig:"POSTGRES_PORT"`
	Password string `envconfig:"POSTGRES_PASSWORD"`
	User     string `envconfig:"POSTGRES_USER"`
	DBName   string `envconfig:"POSTGRES_DBNAME"`
	SSLMode  string `envconfig:"POSTGRES_SSLMODE"`
}

var config Config

func init() {
	var err = godotenv.Load()

	err = envconfig.Process("", &config)
	if err != nil {
		log.Fatalf("parse config error: %s", err.Error())
	}
}

func Get() *Config {
	return &config
}
