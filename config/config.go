package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const minSecretLen = 32

// Config — корневая конфигурация приложения.
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

// LoadConfig читает конфигурацию из переменных окружения, предварительно
// подгрузив .env (если файл есть рядом с рабочим каталогом).
//
// Обязательные секреты (JWT_ACCESS_SECRET / JWT_REFRESH_SECRET) проверяются
// через mustEnv: при отсутствии переменной или слишком коротком/небезопасном
// значении функция паникует на старте — приложение не должно запускаться с
// дефолтными или слабыми секретами.
func LoadConfig() *Config {
	loadDotEnv()

	accessSecret := mustEnv("JWT_ACCESS_SECRET")
	refreshSecret := mustEnv("JWT_REFRESH_SECRET")
	telegramToken := mustEnv("TELEGRAM_TOKEN")

	// Секреты подписи access/refresh токенов обязаны различаться: иначе
	// refresh-токен можно предъявить как access-токен и наоборот (при
	// одинаковом секрете различие держится только на поле token_type, что
	// заметно ослабляет изоляцию типов токенов).
	if accessSecret == refreshSecret {
		panic("config: JWT_ACCESS_SECRET and JWT_REFRESH_SECRET must differ")
	}

	validateSecret("JWT_ACCESS_SECRET", accessSecret)
	validateSecret("JWT_REFRESH_SECRET", refreshSecret)

	return &Config{
		HTTP: HTTPConfig{
			Port: getEnv("HTTP_PORT", "8080"),
		},
		DB: DBConfig{
			URL: getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/health_checker?sslmode=disable"),
		},
		JWT: JWTConfig{
			AccessSecret:  accessSecret,
			RefreshSecret: refreshSecret,
			AccessTTL:     getEnvDuration("JWT_ACCESS_TTL", time.Hour),
			RefreshTTL:    getEnvDuration("JWT_REFRESH_TTL", time.Hour*24*7),
		},
		Telegram: TelegramConfig{
			Token: telegramToken,
		},
		Checker: CheckerConfig{
			NumWorkers: getEnvInt("CHECKER_NUM_WORKERS", 5),
			Limit:      getEnvInt("CHECKER_LIMIT", 100),
			QueueSize:  getEnvInt("CHECKER_QUEUE_SIZE", 100),
		},
	}
}

// loadDotEnv подгружает переменные из .env-файла. Отсутствие файла — не
// ошибка (в проде переменные приходят из окружения). Реальную ошибку чтения
// существующего .env логируем, но не роняем приложение: для секретов ниже
// всё равно сработает mustEnv.
func loadDotEnv() {
	err := godotenv.Load()
	if err == nil {
		return
	}
	if os.IsNotExist(err) {
		return
	}
	fmt.Fprintf(os.Stderr, "config: failed to load .env: %v\n", err)
}

// validateSecret проверяет длину и «качество» секрета.
func validateSecret(name, value string) {
	// Уже проверено, что value != "", но оставляем защиту от прямого вызова.
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		panic(fmt.Sprintf("config: %s must not be blank", name))
	}

	if len(value) < minSecretLen {
		panic(fmt.Sprintf(
			"config: %s must be at least %d bytes long, got %d; generate one with `openssl rand -base64 48`",
			name, minSecretLen, len(value),
		))
	}

	// Отсеиваем очевидно слабые/шаблонные значения, чтобы в проде не оказался
	// плейсхолдер вроде "нужно доделать" или "changeme".
	lower := strings.ToLower(trimmed)
	for _, weak := range []string{"changeme", "change_me", "secret", "password", "jwt_secret"} {
		if lower == weak {
			panic(fmt.Sprintf("config: %s uses an insecure placeholder value", name))
		}
	}
}

// mustEnv возвращает значение переменной окружения или паникует, если она
// не задана либо пуста. Используется для обязательных параметров.
func mustEnv(key string) string {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		panic(fmt.Sprintf("config: required environment variable %s is not set", key))
	}
	return value
}

func getEnv(key, defaultValue string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		if n, err := strconv.Atoi(value); err == nil {
			return n
		}
		fmt.Fprintf(os.Stderr, "config: invalid int for %s=%q, using default %d\n", key, value, defaultValue)
	}
	return defaultValue
}

// getEnvDuration читает duration в формате time.ParseDuration ("15m", "168h").
// При ошибке парсинга логируем и возвращаем значение по умолчанию.
func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			if d <= 0 {
				fmt.Fprintf(os.Stderr, "config: %s must be positive, using default %s\n", key, defaultValue)
				return defaultValue
			}
			return d
		}
		fmt.Fprintf(os.Stderr, "config: invalid duration for %s=%q, using default %s\n", key, value, defaultValue)
	}
	return defaultValue
}
