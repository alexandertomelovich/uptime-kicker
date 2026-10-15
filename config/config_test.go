// Тесты для загрузки конфигурации (config.go).
// LoadConfig читает переменные окружения и паникует на небезопасных/отсутствующих
// секретах, поэтому для каждого сценария выставляем env через t.Setenv (он сам
// восстанавливает исходное окружение) и ловим panic через recover.
package config

import (
	"testing"
	"time"
)

// setValidSecrets выставляет минимально корректный набор обязательных переменных.
func setValidSecrets(t *testing.T) {
	t.Helper()
	t.Setenv("JWT_ACCESS_SECRET", "access-secret-access-secret-access-123456")
	t.Setenv("JWT_REFRESH_SECRET", "refresh-secret-refresh-secret-refresh-12345")
	t.Setenv("TELEGRAM_TOKEN", "telegram-token")
}

// Успешная загрузка с явно заданными переменными окружения.
func TestLoadConfig_Success(t *testing.T) {
	setValidSecrets(t)
	t.Setenv("HTTP_PORT", "9090")
	t.Setenv("CHECKER_NUM_WORKERS", "7")
	t.Setenv("CHECKER_LIMIT", "42")
	t.Setenv("CHECKER_QUEUE_SIZE", "11")
	t.Setenv("JWT_ACCESS_TTL", "15m")
	t.Setenv("JWT_REFRESH_TTL", "168h")

	cfg := LoadConfig()

	if cfg.HTTP.Port != "9090" {
		t.Errorf("Port = %q, want 9090", cfg.HTTP.Port)
	}
	if cfg.Checker.NumWorkers != 7 {
		t.Errorf("NumWorkers = %d, want 7", cfg.Checker.NumWorkers)
	}
	if cfg.Checker.Limit != 42 {
		t.Errorf("Limit = %d, want 42", cfg.Checker.Limit)
	}
	if cfg.Checker.QueueSize != 11 {
		t.Errorf("QueueSize = %d, want 11", cfg.Checker.QueueSize)
	}
	if cfg.JWT.AccessTTL != 15*time.Minute {
		t.Errorf("AccessTTL = %v, want 15m", cfg.JWT.AccessTTL)
	}
	if cfg.JWT.RefreshTTL != 168*time.Hour {
		t.Errorf("RefreshTTL = %v, want 168h", cfg.JWT.RefreshTTL)
	}
}

// При отсутствии необязательных переменных подставляются значения по умолчанию.
func TestLoadConfig_Defaults(t *testing.T) {
	setValidSecrets(t)

	cfg := LoadConfig()

	if cfg.HTTP.Port != "8080" {
		t.Errorf("порт по умолчанию = %q, want 8080", cfg.HTTP.Port)
	}
	if cfg.Checker.NumWorkers != 5 {
		t.Errorf("NumWorkers по умолчанию = %d, want 5", cfg.Checker.NumWorkers)
	}
	if cfg.Checker.Limit != 100 {
		t.Errorf("Limit по умолчанию = %d, want 100", cfg.Checker.Limit)
	}
	if cfg.Checker.QueueSize != 100 {
		t.Errorf("QueueSize по умолчанию = %d, want 100", cfg.Checker.QueueSize)
	}
	if cfg.JWT.AccessTTL != time.Hour {
		t.Errorf("AccessTTL по умолчанию = %v, want 1h", cfg.JWT.AccessTTL)
	}
}

// Некорректный int для необязательной переменной -> значение по умолчанию.
func TestGetEnvInt_InvalidUsesDefault(t *testing.T) {
	t.Setenv("SOME_INT", "not-a-number")
	if got := getEnvInt("SOME_INT", 99); got != 99 {
		t.Errorf("getEnvInt = %d, want 99", got)
	}
}

func TestGetEnvInt_Valid(t *testing.T) {
	t.Setenv("SOME_INT", "5")
	if got := getEnvInt("SOME_INT", 99); got != 5 {
		t.Errorf("getEnvInt = %d, want 5", got)
	}
}

// Некорректный duration -> значение по умолчанию.
func TestGetEnvDuration_InvalidUsesDefault(t *testing.T) {
	t.Setenv("SOME_DUR", "abc")
	if got := getEnvDuration("SOME_DUR", time.Hour); got != time.Hour {
		t.Errorf("getEnvDuration = %v, want 1h", got)
	}
}

// Неположительный duration -> значение по умолчанию.
func TestGetEnvDuration_NonPositiveUsesDefault(t *testing.T) {
	t.Setenv("SOME_DUR", "-5m")
	if got := getEnvDuration("SOME_DUR", time.Hour); got != time.Hour {
		t.Errorf("getEnvDuration = %v, want 1h", got)
	}
}

// getEnv возвращает значение, если оно задано, иначе default.
func TestGetEnv(t *testing.T) {
	t.Setenv("SOME_STR", "value")
	if got := getEnv("SOME_STR", "def"); got != "value" {
		t.Errorf("getEnv = %q, want value", got)
	}
	if got := getEnv("MISSING_STR_XYZ", "def"); got != "def" {
		t.Errorf("getEnv = %q, want def", got)
	}
}

// --- Паники на небезопасной конфигурации ---

func assertPanics(t *testing.T, name string, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s: ожидалась паника", name)
		}
	}()
	fn()
}

// loadConfigPanics оборачивает LoadConfig (возвращающую значение) в func().
func loadConfigPanics() { _ = LoadConfig() }

// Отсутствие обязательного секрета -> паника.
func TestLoadConfig_MissingAccessSecretPanics(t *testing.T) {
	t.Setenv("JWT_ACCESS_SECRET", "")
	t.Setenv("JWT_REFRESH_SECRET", "refresh-secret-refresh-secret-refresh-12345")
	t.Setenv("TELEGRAM_TOKEN", "telegram-token")

	assertPanics(t, "missing JWT_ACCESS_SECRET", loadConfigPanics)
}

// Access и refresh секреты совпадают -> паника.
func TestLoadConfig_SameSecretsPanics(t *testing.T) {
	secret := "same-secret-same-secret-same-secret-123456"
	t.Setenv("JWT_ACCESS_SECRET", secret)
	t.Setenv("JWT_REFRESH_SECRET", secret)
	t.Setenv("TELEGRAM_TOKEN", "telegram-token")

	assertPanics(t, "equal secrets", loadConfigPanics)
}

// Слишком короткий секрет -> паника.
func TestLoadConfig_ShortSecretPanics(t *testing.T) {
	t.Setenv("JWT_ACCESS_SECRET", "short")
	t.Setenv("JWT_REFRESH_SECRET", "refresh-secret-refresh-secret-refresh-12345")
	t.Setenv("TELEGRAM_TOKEN", "telegram-token")

	assertPanics(t, "short access secret", loadConfigPanics)
}

// Секрет-плейсхолдер -> паника.
func TestLoadConfig_PlaceholderSecretPanics(t *testing.T) {
	t.Setenv("JWT_ACCESS_SECRET", "changeme")
	t.Setenv("JWT_REFRESH_SECRET", "refresh-secret-refresh-secret-refresh-12345")
	t.Setenv("TELEGRAM_TOKEN", "telegram-token")

	assertPanics(t, "placeholder access secret", loadConfigPanics)
}

// Пустой telegram-токен -> паника.
func TestLoadConfig_MissingTelegramTokenPanics(t *testing.T) {
	setValidSecrets(t)
	t.Setenv("TELEGRAM_TOKEN", "")

	assertPanics(t, "missing TELEGRAM_TOKEN", loadConfigPanics)
}
