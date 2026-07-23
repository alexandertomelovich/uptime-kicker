package telegram

import (
	"context"
	"health_checker/internal/notifier"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Sender struct {
	bot *tgbotapi.BotAPI
}

func NewSender(bot *tgbotapi.BotAPI) *Sender {
	return &Sender{
		bot: bot,
	}
}

func (s *Sender) Send(ctx context.Context, message notifier.Message) error {
	msg := tgbotapi.NewMessage(message.ChatID, message.Text)
	msg.ParseMode = message.ParseMode

	_, err := s.bot.Send(msg)
	return err
}	