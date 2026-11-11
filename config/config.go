package config

import (
	"log"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	App     AppConfig
	DBDSN   Postgres
	SMTP    SMTP
	Google  Google
	Storage Storage
}

type Google struct {
	ClientID string `envconfig:"GOOGLE_CLIENT_ID"`
}

type Storage struct {
	Driver             string `envconfig:"STORAGE_DRIVER"` // firebase | local
	FirebaseBucket     string `envconfig:"FIREBASE_STORAGE_BUCKET"`
	FirebaseCredential string `envconfig:"FIREBASE_SERVICE_ACCOUNT_JSON"` // JSON or base64 of it
	LocalDir           string `envconfig:"LOCAL_STORAGE_DIR" default:"./.data/media"`
	PublicAPIURL       string `envconfig:"APP_PUBLIC_API_URL"`
}

type SMTP struct {
	Host     string `envconfig:"SMTP_HOST"`
	Port     string `envconfig:"SMTP_PORT"`
	User     string `envconfig:"SMTP_USER"`
	Password string `envconfig:"SMTP_PASSWORD"`
	From     string `envconfig:"SMTP_FROM"`
}

type AppConfig struct {
	Port   int    `envconfig:"APP_PORT"`
	Name   string `envconfig:"APP_NAME"`
	ENV    string `envconfig:"APP_ENV"`
	Prefix string `envconfig:"APP_PREFIX"`
	APIKey string `envconfig:"APP_API_KEY"`
	WebURL string `envconfig:"APP_WEB_URL"`
	// ClientIPHeader carries the caller's IP behind the proxy (Railway sets X-Real-IP).
	ClientIPHeader string `envconfig:"APP_CLIENT_IP_HEADER" default:"X-Real-IP"`
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
