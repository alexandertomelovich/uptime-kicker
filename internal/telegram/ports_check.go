package telegram

import (
	"health_checker/internal/notifier"
	"health_checker/internal/repository"
	"health_checker/internal/service"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Проверки соответствия конкретных реализаций интерфейсам бота.
// Если сигнатуры разойдутся — ошибка на этапе компиляции.
var (
	_ UserProvider    = (*repository.UserRepository)(nil)
	_ SiteService     = (*service.SiteService)(nil)
	_ BotAPI          = (*tgbotapi.BotAPI)(nil)
	_ notifier.Sender = (*Sender)(nil)
)
