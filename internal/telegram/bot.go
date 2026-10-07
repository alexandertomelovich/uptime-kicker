package telegram

import (
	"context"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// pollTimeout — таймаут long-polling в секундах для getUpdates.
const pollTimeout = 30

// Bot принимает входящие обновления Telegram (long-polling) и передаёт их
// в Handler для обработки. Работает до отмены контекста.
type Bot struct {
	api     *tgbotapi.BotAPI
	handler *Handler
}

func NewBot(api *tgbotapi.BotAPI, handler *Handler) *Bot {
	return &Bot{api: api, handler: handler}
}

// Run запускает цикл приёма сообщений (long-polling) и блокируется до отмены
// ctx. При отмене корректно останавливает получение обновлений.
func (b *Bot) Run(ctx context.Context) error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = pollTimeout

	updates := b.api.GetUpdatesChan(u)

	for {
		select {
		case <-ctx.Done():
			b.api.StopReceivingUpdates()
			return ctx.Err()
		case update, ok := <-updates:
			if !ok {
				// Канал закрыт после StopReceivingUpdates.
				return nil
			}
			b.handleUpdate(ctx, update)
		}
	}
}

// handleUpdate безопасно обрабатывает одно обновление: паника в хендлере не
// должна ронять весь цикл приёма сообщений.
func (b *Bot) handleUpdate(ctx context.Context, update tgbotapi.Update) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("telegram: panic while handling update: %v", r)
		}
	}()

	switch {
	case update.Message != nil:
		b.handler.HandleMessage(ctx, update.Message)
	case update.CallbackQuery != nil:
		b.handler.HandleCallback(ctx, update.CallbackQuery)
	}
}
