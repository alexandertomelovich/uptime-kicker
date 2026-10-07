package telegram

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"health_checker/internal/domain"
	"health_checker/internal/service"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
)

// Handler обрабатывает входящие сообщения и нажатия инлайн-кнопок.
type Handler struct {
	api   *tgbotapi.BotAPI
	users UserProvider
	sites SiteService
}

func NewHandler(api *tgbotapi.BotAPI, users UserProvider, sites SiteService) *Handler {
	if api == nil {
		panic("telegram.Handler: api is nil")
	}
	if users == nil {
		panic("telegram.Handler: users is nil")
	}
	if sites == nil {
		panic("telegram.Handler: sites is nil")
	}
	return &Handler{api: api, users: users, sites: sites}
}

// resolveUser находит внутреннего пользователя по telegram_id.
func (h *Handler) resolveUser(ctx context.Context, chatID int64, telegramID int64) (domain.User, bool) {
	user, err := h.users.GetByTelegramID(ctx, telegramID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			h.reply(chatID, "❌ Аккаунт не найден.\n\nСначала зарегистрируйтесь в веб-приложении, указав ваш Telegram ID: "+fmt.Sprint(telegramID), nil)
			return domain.User{}, false
		}
		log.Printf("telegram: failed to resolve user by telegram_id=%d: %v", telegramID, err)
		h.reply(chatID, "⚠️ Внутренняя ошибка. Попробуйте позже.", nil)
		return domain.User{}, false
	}
	return user, true
}

// HandleMessage обрабатывает текстовое сообщение (команды).
func (h *Handler) HandleMessage(ctx context.Context, msg *tgbotapi.Message) {
	if msg == nil || msg.From == nil {
		return
	}
	chatID := msg.Chat.ID
	telegramID := msg.From.ID

	if !msg.IsCommand() {
		h.reply(chatID, "Я понимаю только команды. Список: /help", nil)
		return
	}

	command := msg.Command()
	args := strings.Fields(msg.CommandArguments())

	// /start и /help доступны без привязки аккаунта.
	switch command {
	case "start":
		h.reply(chatID, welcomeText, nil)
		return
	case "help":
		h.reply(chatID, helpText, nil)
		return
	}

	user, ok := h.resolveUser(ctx, chatID, telegramID)
	if !ok {
		return
	}

	switch command {
	case "add":
		h.cmdAdd(ctx, chatID, user.ID, args)
	case "list":
		h.cmdList(ctx, chatID, user.ID)
	case "status":
		h.cmdStatus(ctx, chatID, user.ID, args)
	case "remove":
		h.cmdRemove(ctx, chatID, user.ID, args)
	case "verify":
		h.cmdVerify(ctx, chatID, user.ID, args)
	case "stats":
		h.cmdStats(ctx, chatID, user.ID)
	default:
		h.reply(chatID, "Неизвестная команда. Список: /help", nil)
	}
}

func (h *Handler) cmdAdd(ctx context.Context, chatID int64, userID uuid.UUID, args []string) {
	if len(args) == 0 {
		h.reply(chatID, "Использование: /add <url> [name]", nil)
		return
	}

	url := args[0]
	name := url
	if len(args) > 1 {
		name = strings.Join(args[1:], " ")
	}

	site, err := h.sites.Create(ctx, service.CreateSiteRequest{
		Url:                  url,
		Name:                 name,
		CheckIntervalSeconds: 0, // сервис подставит минимум (30 сек)
	}, userID)
	if err != nil {
		h.reply(chatID, "❌ Не удалось добавить сайт: "+humanizeError(err), nil)
		return
	}

	text := fmt.Sprintf(
		"✅ Сайт добавлен!\n\n%s %s\nID: %s\n\n"+
			"⚠️ Для запуска проверок подтвердите владение сайтом:\n"+
			"разместите на нём файл с содержимым-токеном и вызовите /verify <id> <token>.\n\n"+
			"Токен: %s",
		statusEmoji(site.Status), site.Name, site.ID, site.VerificationToken,
	)
	h.reply(chatID, text, nil)
}

func (h *Handler) cmdList(ctx context.Context, chatID int64, userID uuid.UUID) {
	sites, err := h.sites.GetByUserID(ctx, userID)
	if err != nil {
		h.reply(chatID, "⚠️ Не удалось получить список: "+humanizeError(err), nil)
		return
	}

	summaries := make([]siteSummary, len(sites))
	for i, s := range sites {
		summaries[i] = siteSummary{
			ID:     s.ID.String(),
			Name:   s.Name,
			Status: string(s.Status),
		}
	}

	kb := siteListKeyboard(summaries)
	h.reply(chatID, formatSiteList(sites), &kb)
}

func (h *Handler) cmdStatus(ctx context.Context, chatID int64, userID uuid.UUID, args []string) {
	if len(args) == 0 {
		h.reply(chatID, "Использование: /status <id>", nil)
		return
	}

	site, ok := h.loadOwnedSite(ctx, chatID, userID, args[0])
	if !ok {
		return
	}

	kb := siteDetailKeyboard(site.ID.String(), site.IsActive)
	h.reply(chatID, formatSiteDetail(site), &kb)
}

func (h *Handler) cmdRemove(ctx context.Context, chatID int64, userID uuid.UUID, args []string) {
	if len(args) == 0 {
		h.reply(chatID, "Использование: /remove <id>", nil)
		return
	}

	site, ok := h.loadOwnedSite(ctx, chatID, userID, args[0])
	if !ok {
		return
	}

	kb := confirmRemoveKeyboard(site.ID.String())
	h.reply(chatID, fmt.Sprintf("Удалить сайт «%s» (%s)?", site.Name, site.Url), &kb)
}

func (h *Handler) cmdVerify(ctx context.Context, chatID int64, userID uuid.UUID, args []string) {
	if len(args) < 2 {
		h.reply(chatID, "Использование: /verify <id> <token>", nil)
		return
	}

	id, err := uuid.Parse(args[0])
	if err != nil {
		h.reply(chatID, "❌ Некорректный ID сайта.", nil)
		return
	}

	// Загружаем сайт, чтобы дать понятную обратную связь и проверить владение
	// до обращения к сервису (сервис тоже проверяет принадлежность).
	site, err := h.sites.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			h.reply(chatID, "❌ Сайт не найден.", nil)
			return
		}
		log.Printf("telegram: cmdVerify GetByID: %v", err)
		h.reply(chatID, "⚠️ Внутренняя ошибка.", nil)
		return
	}
	if site.UserID != userID {
		h.reply(chatID, "❌ Этот сайт вам не принадлежит.", nil)
		return
	}

	token := args[1]
	if err := h.sites.VerifySite(ctx, id, userID, token); err != nil {
		h.reply(chatID, "❌ Не удалось подтвердить сайт: "+humanizeError(err), nil)
		return
	}

	kb := siteDetailKeyboard(id.String(), true)
	h.reply(chatID, fmt.Sprintf("✅ Сайт «%s» успешно подтверждён и активен. Проверки запущены.", site.Name), &kb)
}

func (h *Handler) cmdStats(ctx context.Context, chatID int64, userID uuid.UUID) {
	stats, err := h.sites.GetSiteStats(ctx, userID)
	if err != nil {
		h.reply(chatID, "⚠️ Не удалось получить статистику: "+humanizeError(err), nil)
		return
	}
	h.reply(chatID, formatStats(stats), nil)
}

// loadOwnedSite парсит id и загружает сайт, проверяя принадлежность пользователю.
func (h *Handler) loadOwnedSite(ctx context.Context, chatID int64, userID uuid.UUID, rawID string) (domain.Site, bool) {
	id, err := uuid.Parse(rawID)
	if err != nil {
		h.reply(chatID, "❌ Некорректный ID сайта.", nil)
		return domain.Site{}, false
	}

	site, err := h.sites.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			h.reply(chatID, "❌ Сайт не найден.", nil)
			return domain.Site{}, false
		}
		log.Printf("telegram: loadOwnedSite: %v", err)
		h.reply(chatID, "⚠️ Внутренняя ошибка.", nil)
		return domain.Site{}, false
	}

	if site.UserID != userID {
		h.reply(chatID, "❌ Этот сайт вам не принадлежит.", nil)
		return domain.Site{}, false
	}
	return site, true
}

// reply отправляет текстовое сообщение, опционально с инлайн-клавиатурой.
func (h *Handler) reply(chatID int64, text string, kb *tgbotapi.InlineKeyboardMarkup) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "HTML"
	if kb != nil {
		msg.ReplyMarkup = *kb
	}

	if _, err := h.api.Send(msg); err != nil {
		log.Printf("telegram: failed to send message to chat %d: %v", chatID, err)
	}
}

// humanizeError превращает доменную ошибку в понятный пользователю текст.
func humanizeError(err error) string {
	switch {
	case errors.Is(err, domain.ErrSiteNotBelongUser):
		return "сайт вам не принадлежит"
	case errors.Is(err, domain.ErrNotFound):
		return "не найдено"
	case errors.Is(err, domain.ErrSiteAlreadyVerified):
		return "сайт уже подтверждён"
	case errors.Is(err, domain.ErrInvalidToken):
		return "неверный токен подтверждения"
	default:
		return "внутренняя ошибка"
	}
}
