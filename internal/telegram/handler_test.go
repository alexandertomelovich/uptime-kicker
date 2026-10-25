// Тесты для Telegram-хендлера (handler.go, callback.go).
//
// Реальная сеть не используется: Telegram-клиент подменяется моком BotAPI,
// который запоминает отправленные сообщения и подтверждённые callback-и.
// Провайдер пользователя и сервис сайтов также подменяются ручными моками,
// что позволяет точечно проверять ветвления и mapping доменных ошибок в
// пользовательский текст.
package telegram

import (
	"context"
	"strings"
	"sync"

	"health_checker/internal/domain"
	"health_checker/internal/service"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
)

// --- Моки ---

// sentMessage — отправленное моком сообщение (для последующих проверок).
type sentMessage struct {
	ChatID      int64
	Text        string
	ParseMode   string
	ReplyMarkup interface{}
}

// mockBotAPI реализует BotAPI, запоминая вызовы Send/Request.
type mockBotAPI struct {
	mu sync.Mutex

	sent      []sentMessage
	requested []tgbotapi.Chattable

	sendErr    error
	requestErr error
}

func (m *mockBotAPI) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendErr != nil {
		return tgbotapi.Message{}, m.sendErr
	}
	if msg, ok := c.(tgbotapi.MessageConfig); ok {
		m.sent = append(m.sent, sentMessage{
			ChatID:      msg.ChatID,
			Text:        msg.Text,
			ParseMode:   msg.ParseMode,
			ReplyMarkup: msg.ReplyMarkup,
		})
	}
	return tgbotapi.Message{}, nil
}

func (m *mockBotAPI) Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requested = append(m.requested, c)
	if m.requestErr != nil {
		return nil, m.requestErr
	}
	return &tgbotapi.APIResponse{Ok: true}, nil
}

// lastText возвращает текст последнего отправленного сообщения.
func (m *mockBotAPI) lastText() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.sent) == 0 {
		return ""
	}
	return m.sent[len(m.sent)-1].Text
}

func (m *mockBotAPI) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sent)
}

func (m *mockBotAPI) requestCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.requested)
}

// mockUsers реализует UserProvider.
type mockUsers struct {
	user domain.User
	err  error
}

func (m *mockUsers) GetByTelegramID(ctx context.Context, telegramID int64) (domain.User, error) {
	if m.err != nil {
		return domain.User{}, m.err
	}
	return m.user, nil
}

// mockSites реализует SiteService. Каждое поле — опциональная функция-заглушка;
// если не задана, возвращается безопасный дефолт.
type mockSites struct {
	createFn  func(ctx context.Context, req service.CreateSiteRequest, userID uuid.UUID) (domain.Site, error)
	listFn    func(ctx context.Context, userID uuid.UUID) ([]domain.SiteResponse, error)
	getByIDFn func(ctx context.Context, id, userID uuid.UUID) (domain.SiteResponse, error)
	statsFn   func(ctx context.Context, userID uuid.UUID) (domain.SiteStats, error)
	deleteFn  func(ctx context.Context, id, userID uuid.UUID) error
	updateFn  func(ctx context.Context, update domain.SiteUpdate) (domain.SiteResponse, error)

	lastCreate service.CreateSiteRequest
	lastUpdate domain.SiteUpdate
	lastDelete uuid.UUID
}

func (m *mockSites) Create(ctx context.Context, req service.CreateSiteRequest, userID uuid.UUID) (domain.Site, error) {
	m.lastCreate = req
	if m.createFn != nil {
		return m.createFn(ctx, req, userID)
	}
	return domain.Site{ID: uuid.New(), UserID: userID, Name: req.Name, Url: req.Url}, nil
}

func (m *mockSites) GetByUserID(ctx context.Context, userID uuid.UUID) ([]domain.SiteResponse, error) {
	if m.listFn != nil {
		return m.listFn(ctx, userID)
	}
	return nil, nil
}

func (m *mockSites) GetByID(ctx context.Context, id, userID uuid.UUID) (domain.SiteResponse, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id, userID)
	}
	return domain.SiteResponse{ID: id}, nil
}

func (m *mockSites) GetSiteStats(ctx context.Context, userID uuid.UUID) (domain.SiteStats, error) {
	if m.statsFn != nil {
		return m.statsFn(ctx, userID)
	}
	return domain.SiteStats{}, nil
}

func (m *mockSites) Delete(ctx context.Context, id, userID uuid.UUID) error {
	m.lastDelete = id
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id, userID)
	}
	return nil
}

func (m *mockSites) Update(ctx context.Context, update domain.SiteUpdate) (domain.SiteResponse, error) {
	m.lastUpdate = update
	if m.updateFn != nil {
		return m.updateFn(ctx, update)
	}
	return domain.SiteResponse{ID: update.ID}, nil
}

// --- Хелперы сборки ---

// newTestHandler собирает Handler с моками. Возвращает хендлер и моки.
func newTestHandler(users *mockUsers, sites *mockSites) (*Handler, *mockBotAPI) {
	api := &mockBotAPI{}
	return NewHandler(api, users, sites), api
}

// registeredUser — пользователь, успешно резолвящийся по telegram_id.
func registeredUser(telegramID int64) *mockUsers {
	return &mockUsers{user: domain.User{ID: uuid.New(), TelegramID: telegramID, Name: "Alice"}}
}

// cmdMessage строит командное сообщение (e.g. "/add https://x").
func cmdMessage(telegramID, chatID int64, text string) *tgbotapi.Message {
	return &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: chatID},
		From: &tgbotapi.User{ID: telegramID},
		Text: text,
		Entities: []tgbotapi.MessageEntity{
			{Type: "bot_command", Offset: 0, Length: len(strings.SplitN(text, " ", 2)[0])},
		},
	}
}

// textMessage строит обычное (не командное) сообщение.
func textMessage(telegramID, chatID int64, text string) *tgbotapi.Message {
	return &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: chatID},
		From: &tgbotapi.User{ID: telegramID},
		Text: text,
	}
}

// callbackQuery строит нажатие инлайн-кнопки.
func callbackQuery(telegramID, chatID int64, data string) *tgbotapi.CallbackQuery {
	return &tgbotapi.CallbackQuery{
		ID:   "cb-1",
		From: &tgbotapi.User{ID: telegramID},
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{ID: chatID},
		},
		Data: data,
	}
}

const (
	testTelegramID = int64(555)
	testChatID     = int64(999)
)
