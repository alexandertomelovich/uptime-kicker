package telegram

import (
	"context"
	"fmt"

	"health_checker/internal/notifier"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Sender — адаптер Telegram-клиента к интерфейсу notifier.Sender.
// Используется checker-сервисом (алерты о падении сайтов) и UserService
// (welcome-сообщение при регистрации).
//
// Зависит от интерфейса BotAPI, а не от *tgbotapi.BotAPI, чтобы реализацию
// можно было подменять в тестах без сетевых вызовов.
type Sender struct {
	bot BotAPI
}

func NewSender(bot BotAPI) *Sender {
	if bot == nil {
		panic("telegram.Sender: bot is nil")
	}
	return &Sender{
		bot: bot,
	}
}

// Send реализует notifier.Sender: отправляет сообщение в указанный чат.
func (s *Sender) Send(ctx context.Context, message notifier.Message) error {
	msg := tgbotapi.NewMessage(message.ChatID, message.Text)
	msg.ParseMode = message.ParseMode

	if _, err := s.bot.Send(msg); err != nil {
		return fmt.Errorf("telegram.Sender.Send: %w", err)
	}
	return nil
}
