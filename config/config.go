package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTP     HTTPConfig
	DB       DBConfig
	JWT      JWTConfig
	Telegram TelegramConfig
	Checker  CheckerConfig
}

type HTTPConfig struct {
	Port string
}

type DBConfig struct {
	URL string
}

type JWTConfig struct {
	AccessSecret  string
	RefreshSecret string
	AccessTTL     time.Duration
	RefreshTTL    time.Duration
}

type TelegramConfig struct {
	Token string
}

type CheckerConfig struct {
	NumWorkers int
	Limit      int
	QueueSize  int
}

func LoadConfig() *Config {
	return &Config{
		HTTP: HTTPConfig{
			Port: getEnv("HTTP_PORT", "8080"),
		},
		DB: DBConfig{
			URL: getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/health_checker?sslmode=disable"),
		},
		JWT: JWTConfig{
			AccessSecret:  getEnv("JWT_ACCESS_SECRET", "нужно доделать"),
			RefreshSecret: getEnv("JWT_REFRESH_SECRET", "нужно доделать"),
			AccessTTL:     time.Hour * 24,
			RefreshTTL:    time.Hour * 24 * 7,
		},
		Telegram: TelegramConfig{
			Token: getEnv("TELEGRAM_TOKEN", ""),
		},
		Checker: CheckerConfig{
			NumWorkers: getEnvInt("CHECKER_NUM_WORKERS", 5),
			Limit:      getEnvInt("CHECKER_LIMIT", 100),
			QueueSize:  getEnvInt("CHECKER_QUEUE_SIZE", 100),
		},
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if n, err := strconv.Atoi(value); err == nil {
			return n
		}
	}
	return defaultValue
}
