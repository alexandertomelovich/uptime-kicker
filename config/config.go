package config

import (
	"os"
	"time"
)

type Config struct {
	JWT JWTConfig
}

type JWTConfig struct {
	AccessSecret  string
	RefreshSecret string
	AccessTTL     time.Duration
	RefreshTTL    time.Duration
}

func LoadConfig() *Config {
	return &Config{
		JWT: JWTConfig{
			AccessSecret:  getEnv("JWT_ACCESS_SECRET", "нужно доделать"),
			RefreshSecret: getEnv("JWT_REFRESH_SECRET", "нужно доделать"),
			AccessTTL:     time.Hour * 24,
			RefreshTTL:    time.Hour * 24 * 7,
		},
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}