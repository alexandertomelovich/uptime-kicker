package telegram

import (
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// callback data формируются как "<action>:<siteID>".
const (
	actionStatus  = "status"
	actionRemove  = "remove"
	actionToggle  = "toggle"
	actionConfirm = "confirm"
)

func callbackData(action, siteID string) string {
	return fmt.Sprintf("%s:%s", action, siteID)
}

// siteSummary — минимальные данные для построения клавиатуры списка.
type siteSummary struct {
	ID     string
	Name   string
	Status string
}

// siteListKeyboard строит инлайн-клавиатуру со кнопками для каждого сайта.
func siteListKeyboard(sites []siteSummary) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, s := range sites {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				statusEmojiText(s.Status)+" "+shorten(s.Name, 24),
				callbackData(actionStatus, s.ID),
			),
		))
	}

	if len(rows) == 0 {
		return tgbotapi.InlineKeyboardMarkup{}
	}

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// siteDetailKeyboard — кнопки действий для конкретного сайта.
func siteDetailKeyboard(siteID string, isActive bool) tgbotapi.InlineKeyboardMarkup {
	toggleText := "⏸ Остановить"
	if !isActive {
		toggleText = "▶️ Запустить"
	}

	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(toggleText, callbackData(actionToggle, siteID)),
			tgbotapi.NewInlineKeyboardButtonData("🔄 Обновить", callbackData(actionStatus, siteID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🗑 Удалить", callbackData(actionRemove, siteID)),
		),
	)
}

// confirmRemoveKeyboard — подтверждение удаления сайта.
func confirmRemoveKeyboard(siteID string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Да, удалить", callbackData(actionConfirm, siteID)),
			tgbotapi.NewInlineKeyboardButtonData("↩️ Отмена", callbackData(actionStatus, siteID)),
		),
	)
}

func statusEmojiText(status string) string {
	switch status {
	case "up":
		return "🟢"
	case "down":
		return "🔴"
	case "pending":
		return "🟡"
	default:
		return "⚪"
	}
}

// shorten обрезает строку по рунам, добавляя многоточие.
func shorten(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 1 {
		return string(runes[:max])
	}
	return string(runes[:max-1]) + "…"
}
