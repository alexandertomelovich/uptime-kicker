package telegram

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"health_checker/internal/domain"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
)

// HandleCallback обрабатывает нажатия инлайн-кнопок.
// Формат callback data: "<action>:<siteID>", где action — одно из
// actionStatus/actionRemove/actionToggle/actionConfirm.
func (h *Handler) HandleCallback(ctx context.Context, cb *tgbotapi.CallbackQuery) {
	if cb == nil || cb.From == nil {
		return
	}

	// Подтверждаем нажатие, чтобы у клиента не крутились «часики».
	if err := h.answerCallback(cb.ID); err != nil {
		log.Printf("telegram: failed to answer callback %s: %v", cb.ID, err)
	}

	action, siteID, ok := parseCallbackData(cb.Data)
	if !ok {
		log.Printf("telegram: malformed callback data: %q", cb.Data)
		return
	}

	chatID := cb.Message.Chat.ID
	telegramID := cb.From.ID

	user, found := h.resolveUser(ctx, chatID, telegramID)
	if !found {
		return
	}

	id, err := uuid.Parse(siteID)
	if err != nil {
		h.reply(chatID, "❌ Некорректный ID сайта.", nil)
		return
	}

	// Загружаем сайт и проверяем владение.
	site, err := h.sites.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			h.reply(chatID, "❌ Сайт не найден (возможно, уже удалён).", nil)
			return
		}
		log.Printf("telegram: callback GetByID: %v", err)
		h.reply(chatID, "⚠️ Внутренняя ошибка.", nil)
		return
	}
	if site.UserID != user.ID {
		h.reply(chatID, "❌ Этот сайт вам не принадлежит.", nil)
		return
	}

	switch action {
	case actionStatus:
		kb := siteDetailKeyboard(site.ID.String(), site.IsActive)
		h.reply(chatID, formatSiteDetail(site), &kb)
	case actionRemove:
		kb := confirmRemoveKeyboard(site.ID.String())
		h.reply(chatID, fmt.Sprintf("Удалить сайт «%s» (%s)?", site.Name, site.Url), &kb)
	case actionConfirm:
		h.confirmRemove(ctx, chatID, user.ID, site)
	case actionToggle:
		h.toggleSite(ctx, chatID, user.ID, site)
	default:
		log.Printf("telegram: unknown callback action: %q", action)
	}
}

// confirmRemove выполняет удаление сайта после подтверждения.
func (h *Handler) confirmRemove(ctx context.Context, chatID int64, userID uuid.UUID, site domain.Site) {
	if err := h.sites.Delete(ctx, site.ID, userID); err != nil {
		h.reply(chatID, "❌ Не удалось удалить: "+humanizeError(err), nil)
		return
	}
	h.reply(chatID, fmt.Sprintf("🗑 Сайт «%s» удалён.", site.Name), nil)
}

// toggleSite переключает активность сайта (пауза/возобновление).
func (h *Handler) toggleSite(ctx context.Context, chatID int64, userID uuid.UUID, site domain.Site) {
	newActive := !site.IsActive

	updated, err := h.sites.Update(ctx, domain.SiteUpdate{
		ID:       site.ID,
		UserID:   userID,
		IsActive: &newActive,
	})
	if err != nil {
		h.reply(chatID, "❌ Не удалось изменить статус: "+humanizeError(err), nil)
		return
	}

	stateText := "возобновлён"
	if !updated.IsActive {
		stateText = "поставлен на паузу"
	}

	kb := siteDetailKeyboard(updated.ID.String(), updated.IsActive)
	h.reply(chatID, fmt.Sprintf("✅ Сайт «%s» %s.", updated.Name, stateText), &kb)
}

// parseCallbackData разбирает строку "<action>:<siteID>".
func parseCallbackData(data string) (action, siteID string, ok bool) {
	parts := strings.SplitN(data, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// answerCallback подтверждает нажатие инлайн-кнопки.
func (h *Handler) answerCallback(callbackID string) error {
	_, err := h.api.Request(tgbotapi.NewCallback(callbackID, ""))
	return err
}
