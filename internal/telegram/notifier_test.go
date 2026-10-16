// Тесты для адаптера notifier.Sender (notifier.go).
// Telegram-клиент подменяется моком BotAPI — сеть не используется.
package telegram

import (
	"context"
	"errors"
	"testing"

	"health_checker/internal/notifier"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Send успешно передаёт сообщение в клиент с корректными полями.
func TestSender_Send_Success(t *testing.T) {
	api := &mockBotAPI{}
	s := NewSender(api)

	err := s.Send(context.Background(), notifier.Message{
		ChatID:    12345,
		Text:      "alert!",
		ParseMode: "HTML",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if api.count() != 1 {
		t.Fatalf("отправлено сообщений = %d, want 1", api.count())
	}
	got := api.sent[0]
	if got.ChatID != 12345 {
		t.Errorf("ChatID = %d, want 12345", got.ChatID)
	}
	if got.Text != "alert!" {
		t.Errorf("Text = %q, want alert!", got.Text)
	}
	if got.ParseMode != "HTML" {
		t.Errorf("ParseMode = %q, want HTML", got.ParseMode)
	}
}

// Ошибка клиента оборачивается и возвращается наверх.
func TestSender_Send_Error(t *testing.T) {
	api := &mockBotAPI{sendErr: errors.New("telegram down")}
	s := NewSender(api)

	err := s.Send(context.Background(), notifier.Message{ChatID: 1, Text: "x"})
	if err == nil {
		t.Fatal("ожидалась ошибка отправки")
	}
	if !errors.Is(err, api.sendErr) {
		t.Errorf("ошибка должна оборачивать исходную: %v", err)
	}
}

// NewSender паникует на nil-клиенте.
func TestNewSender_NilPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("ожидалась паника при nil-клиенте")
		}
	}()
	NewSender(nil)
}

// Гарантируем, что tgbotapi.BotAPI совместим с BotAPI (компиляционно),
// и что сам Sender реализует notifier.Sender.
var (
	_ notifier.Sender = (*Sender)(nil)
	_ BotAPI          = (*tgbotapi.BotAPI)(nil)
)
