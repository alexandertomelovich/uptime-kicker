// Тесты обработки входящих команд и сообщений (handler.go).
package telegram

import (
	"context"
	"errors"
	"strings"
	"testing"

	"health_checker/internal/domain"
	"health_checker/internal/service"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
)

// --- HandleMessage: базовая маршрутизация ---

// nil-сообщение и nil From игнорируются (не должно быть паники).
func TestHandleMessage_NilSafe(t *testing.T) {
	h, api := newTestHandler(registeredUser(testTelegramID), &mockSites{})

	h.HandleMessage(context.Background(), nil)
	h.HandleMessage(context.Background(), &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: testChatID}})

	if api.count() != 0 {
		t.Errorf("на nil-сообщения не должно быть ответов, got %d", api.count())
	}
}

// Не-командное сообщение -> подсказка про /help.
func TestHandleMessage_NotACommand(t *testing.T) {
	h, api := newTestHandler(registeredUser(testTelegramID), &mockSites{})

	h.HandleMessage(context.Background(), textMessage(testTelegramID, testChatID, "привет"))

	if !strings.Contains(api.lastText(), "/help") {
		t.Errorf("ожидалась подсказка про /help, got %q", api.lastText())
	}
}

// /start и /help работают без привязки аккаунта (пользователь не резолвится).
func TestHandleMessage_StartAndHelp_NoUserResolution(t *testing.T) {
	users := &mockUsers{err: domain.ErrNotFound} // пользователь не найден
	h, api := newTestHandler(users, &mockSites{})

	h.HandleMessage(context.Background(), cmdMessage(testTelegramID, testChatID, "/start"))
	if !strings.Contains(api.lastText(), "Uptime Kicker") {
		t.Errorf("/start: неожиданный ответ %q", api.lastText())
	}

	h.HandleMessage(context.Background(), cmdMessage(testTelegramID, testChatID, "/help"))
	if !strings.Contains(api.lastText(), "/add") {
		t.Errorf("/help: неожиданный ответ %q", api.lastText())
	}
}

// Неизвестная команда -> подсказка про /help.
func TestHandleMessage_UnknownCommand(t *testing.T) {
	h, api := newTestHandler(registeredUser(testTelegramID), &mockSites{})

	h.HandleMessage(context.Background(), cmdMessage(testTelegramID, testChatID, "/nope"))

	if !strings.Contains(api.lastText(), "Неизвестная команда") {
		t.Errorf("ожидалась подсказка о неизвестной команде, got %q", api.lastText())
	}
}

// Пользователь не найден (ErrNotFound) -> сообщение о необходимости регистрации.
func TestHandleMessage_UserNotFound(t *testing.T) {
	users := &mockUsers{err: domain.ErrNotFound}
	h, api := newTestHandler(users, &mockSites{})

	h.HandleMessage(context.Background(), cmdMessage(testTelegramID, testChatID, "/list"))

	if !strings.Contains(api.lastText(), "Аккаунт не найден") {
		t.Errorf("ожидалось сообщение об отсутствии аккаунта, got %q", api.lastText())
	}
}

// Прочая ошибка резолва пользователя -> внутренняя ошибка.
func TestHandleMessage_UserResolveError(t *testing.T) {
	users := &mockUsers{err: errors.New("db down")}
	h, api := newTestHandler(users, &mockSites{})

	h.HandleMessage(context.Background(), cmdMessage(testTelegramID, testChatID, "/list"))

	if !strings.Contains(api.lastText(), "Внутренняя ошибка") {
		t.Errorf("ожидалась внутренняя ошибка, got %q", api.lastText())
	}
}

// --- /add ---

func TestCmdAdd_WithURLAndName(t *testing.T) {
	sites := &mockSites{
		createFn: func(ctx context.Context, req service.CreateSiteRequest, userID uuid.UUID) (domain.Site, error) {
			return domain.Site{ID: uuid.New(), Name: req.Name, Url: req.Url, VerificationToken: "tok"}, nil
		},
	}
	h, api := newTestHandler(registeredUser(testTelegramID), sites)

	h.HandleMessage(context.Background(), cmdMessage(testTelegramID, testChatID, "/add https://example.com My Site"))

	if sites.lastCreate.Url != "https://example.com" {
		t.Errorf("url = %q", sites.lastCreate.Url)
	}
	if sites.lastCreate.Name != "My Site" {
		t.Errorf("name = %q, want 'My Site'", sites.lastCreate.Name)
	}
	if !strings.Contains(api.lastText(), "Сайт добавлен") {
		t.Errorf("ожидалось подтверждение добавления, got %q", api.lastText())
	}
}

// Без имени имя = url.
func TestCmdAdd_NoNameUsesURL(t *testing.T) {
	sites := &mockSites{}
	h, _ := newTestHandler(registeredUser(testTelegramID), sites)

	h.HandleMessage(context.Background(), cmdMessage(testTelegramID, testChatID, "/add https://example.com"))

	if sites.lastCreate.Name != "https://example.com" {
		t.Errorf("name = %q, want url", sites.lastCreate.Name)
	}
}

// Без аргументов -> подсказка по использованию.
func TestCmdAdd_NoArgs(t *testing.T) {
	h, api := newTestHandler(registeredUser(testTelegramID), &mockSites{})

	h.HandleMessage(context.Background(), cmdMessage(testTelegramID, testChatID, "/add"))

	if !strings.Contains(api.lastText(), "Использование") {
		t.Errorf("ожидалась подсказка по использованию, got %q", api.lastText())
	}
}

// Ошибка создания сайта -> понятное сообщение (humanizeError).
func TestCmdAdd_CreateError(t *testing.T) {
	sites := &mockSites{
		createFn: func(ctx context.Context, req service.CreateSiteRequest, userID uuid.UUID) (domain.Site, error) {
			return domain.Site{}, domain.ErrSiteNotBelongUser
		},
	}
	h, api := newTestHandler(registeredUser(testTelegramID), sites)

	h.HandleMessage(context.Background(), cmdMessage(testTelegramID, testChatID, "/add https://x"))

	if !strings.Contains(api.lastText(), "не принадлежит") {
		t.Errorf("ожидалась ошибка о принадлежности, got %q", api.lastText())
	}
}

// --- /list ---

func TestCmdList_Success(t *testing.T) {
	sites := &mockSites{
		listFn: func(ctx context.Context, userID uuid.UUID) ([]domain.SiteResponse, error) {
			return []domain.SiteResponse{{ID: uuid.New(), Name: "A", Url: "https://a", Status: domain.StatusUp}}, nil
		},
	}
	h, api := newTestHandler(registeredUser(testTelegramID), sites)

	h.HandleMessage(context.Background(), cmdMessage(testTelegramID, testChatID, "/list"))

	if !strings.Contains(api.lastText(), "Ваши сайты") {
		t.Errorf("ожидался список сайтов, got %q", api.lastText())
	}
	// Клавиатура со списком не должна быть пустой.
	if api.sent[len(api.sent)-1].ReplyMarkup == nil {
		t.Error("ожидалась инлайн-клавиатура для списка")
	}
}

func TestCmdList_Error(t *testing.T) {
	sites := &mockSites{
		listFn: func(ctx context.Context, userID uuid.UUID) ([]domain.SiteResponse, error) {
			return nil, errors.New("boom")
		},
	}
	h, api := newTestHandler(registeredUser(testTelegramID), sites)

	h.HandleMessage(context.Background(), cmdMessage(testTelegramID, testChatID, "/list"))

	if !strings.Contains(api.lastText(), "Не удалось получить список") {
		t.Errorf("ожидалась ошибка получения списка, got %q", api.lastText())
	}
}

// --- /status ---

func TestCmdStatus_Success(t *testing.T) {
	siteID := uuid.New()
	user := registeredUser(testTelegramID)
	sites := &mockSites{
		getByIDFn: func(ctx context.Context, id, userID uuid.UUID) (domain.SiteResponse, error) {
			return domain.SiteResponse{ID: id, UserID: user.user.ID, Name: "S", Url: "https://s", Status: domain.StatusUp, IsActive: true}, nil
		},
	}
	h, api := newTestHandler(user, sites)

	h.HandleMessage(context.Background(), cmdMessage(testTelegramID, testChatID, "/status "+siteID.String()))

	if !strings.Contains(api.lastText(), "S") {
		t.Errorf("ожидалась детальная карточка, got %q", api.lastText())
	}
}

func TestCmdStatus_NoArgs(t *testing.T) {
	h, api := newTestHandler(registeredUser(testTelegramID), &mockSites{})

	h.HandleMessage(context.Background(), cmdMessage(testTelegramID, testChatID, "/status"))

	if !strings.Contains(api.lastText(), "Использование") {
		t.Errorf("ожидалась подсказка, got %q", api.lastText())
	}
}

func TestCmdStatus_InvalidID(t *testing.T) {
	h, api := newTestHandler(registeredUser(testTelegramID), &mockSites{})

	h.HandleMessage(context.Background(), cmdMessage(testTelegramID, testChatID, "/status not-a-uuid"))

	if !strings.Contains(api.lastText(), "Некорректный ID") {
		t.Errorf("ожидалась ошибка про ID, got %q", api.lastText())
	}
}

func TestCmdStatus_NotFound(t *testing.T) {
	sites := &mockSites{
		getByIDFn: func(ctx context.Context, id, userID uuid.UUID) (domain.SiteResponse, error) {
			return domain.SiteResponse{}, domain.ErrNotFound
		},
	}
	h, api := newTestHandler(registeredUser(testTelegramID), sites)

	h.HandleMessage(context.Background(), cmdMessage(testTelegramID, testChatID, "/status "+uuid.New().String()))

	if !strings.Contains(api.lastText(), "Сайт не найден") {
		t.Errorf("ожидалось 'Сайт не найден', got %q", api.lastText())
	}
}

// Сайт чужой -> сервис возвращает ErrSiteNotBelongUser.
func TestCmdStatus_NotOwned(t *testing.T) {
	sites := &mockSites{
		getByIDFn: func(ctx context.Context, id, userID uuid.UUID) (domain.SiteResponse, error) {
			return domain.SiteResponse{}, domain.ErrSiteNotBelongUser
		},
	}
	h, api := newTestHandler(registeredUser(testTelegramID), sites)

	h.HandleMessage(context.Background(), cmdMessage(testTelegramID, testChatID, "/status "+uuid.New().String()))

	if !strings.Contains(api.lastText(), "не принадлежит") {
		t.Errorf("ожидалась ошибка принадлежности, got %q", api.lastText())
	}
}

// --- /remove ---

func TestCmdRemove_ShowsConfirmation(t *testing.T) {
	siteID := uuid.New()
	user := registeredUser(testTelegramID)
	sites := &mockSites{
		getByIDFn: func(ctx context.Context, id, userID uuid.UUID) (domain.SiteResponse, error) {
			return domain.SiteResponse{ID: id, UserID: user.user.ID, Name: "S", Url: "https://s"}, nil
		},
	}
	h, api := newTestHandler(user, sites)

	h.HandleMessage(context.Background(), cmdMessage(testTelegramID, testChatID, "/remove "+siteID.String()))

	if !strings.Contains(api.lastText(), "Удалить сайт") {
		t.Errorf("ожидался запрос подтверждения, got %q", api.lastText())
	}
	if api.sent[api.count()-1].ReplyMarkup == nil {
		t.Error("ожидалась клавиатура подтверждения")
	}
}

func TestCmdRemove_NoArgs(t *testing.T) {
	h, api := newTestHandler(registeredUser(testTelegramID), &mockSites{})

	h.HandleMessage(context.Background(), cmdMessage(testTelegramID, testChatID, "/remove"))

	if !strings.Contains(api.lastText(), "Использование") {
		t.Errorf("ожидалась подсказка, got %q", api.lastText())
	}
}

// --- /verify ---

func TestCmdVerify_Success(t *testing.T) {
	siteID := uuid.New()
	user := registeredUser(testTelegramID)
	sites := &mockSites{
		getByIDFn: func(ctx context.Context, id, userID uuid.UUID) (domain.SiteResponse, error) {
			return domain.SiteResponse{ID: id, UserID: user.user.ID, Name: "S"}, nil
		},
	}
	h, api := newTestHandler(user, sites)

	h.HandleMessage(context.Background(), cmdMessage(testTelegramID, testChatID, "/verify "+siteID.String()+" tok"))

	if sites.lastVerify.token != "tok" || sites.lastVerify.id != siteID {
		t.Errorf("verify аргументы = %+v", sites.lastVerify)
	}
	if !strings.Contains(api.lastText(), "успешно подтверждён") {
		t.Errorf("ожидалось подтверждение, got %q", api.lastText())
	}
}

func TestCmdVerify_NoArgs(t *testing.T) {
	h, api := newTestHandler(registeredUser(testTelegramID), &mockSites{})

	h.HandleMessage(context.Background(), cmdMessage(testTelegramID, testChatID, "/verify "+uuid.New().String()))

	if !strings.Contains(api.lastText(), "Использование") {
		t.Errorf("ожидалась подсказка, got %q", api.lastText())
	}
}

func TestCmdVerify_InvalidID(t *testing.T) {
	h, api := newTestHandler(registeredUser(testTelegramID), &mockSites{})

	h.HandleMessage(context.Background(), cmdMessage(testTelegramID, testChatID, "/verify bad tok"))

	if !strings.Contains(api.lastText(), "Некорректный ID") {
		t.Errorf("ожидалась ошибка про ID, got %q", api.lastText())
	}
}

func TestCmdVerify_SiteNotFound(t *testing.T) {
	sites := &mockSites{
		getByIDFn: func(ctx context.Context, id, userID uuid.UUID) (domain.SiteResponse, error) {
			return domain.SiteResponse{}, domain.ErrNotFound
		},
	}
	h, api := newTestHandler(registeredUser(testTelegramID), sites)

	h.HandleMessage(context.Background(), cmdMessage(testTelegramID, testChatID, "/verify "+uuid.New().String()+" tok"))

	if !strings.Contains(api.lastText(), "Сайт не найден") {
		t.Errorf("ожидалось 'Сайт не найден', got %q", api.lastText())
	}
}

// Сайт чужой -> сервис (GetByID) возвращает ErrSiteNotBelongUser, до VerifySite не доходим.
func TestCmdVerify_NotOwned(t *testing.T) {
	sites := &mockSites{
		getByIDFn: func(ctx context.Context, id, userID uuid.UUID) (domain.SiteResponse, error) {
			return domain.SiteResponse{}, domain.ErrSiteNotBelongUser
		},
	}
	h, api := newTestHandler(registeredUser(testTelegramID), sites)

	h.HandleMessage(context.Background(), cmdMessage(testTelegramID, testChatID, "/verify "+uuid.New().String()+" tok"))

	if !strings.Contains(api.lastText(), "не принадлежит") {
		t.Errorf("ожидалась ошибка принадлежности, got %q", api.lastText())
	}
}

func TestCmdVerify_ServiceError(t *testing.T) {
	siteID := uuid.New()
	user := registeredUser(testTelegramID)
	sites := &mockSites{
		getByIDFn: func(ctx context.Context, id, userID uuid.UUID) (domain.SiteResponse, error) {
			return domain.SiteResponse{ID: id, UserID: user.user.ID}, nil
		},
		verifyFn: func(ctx context.Context, id, userID uuid.UUID, token string) error {
			return domain.ErrInvalidToken
		},
	}
	h, api := newTestHandler(user, sites)

	h.HandleMessage(context.Background(), cmdMessage(testTelegramID, testChatID, "/verify "+siteID.String()+" tok"))

	if !strings.Contains(api.lastText(), "Не удалось подтвердить") {
		t.Errorf("ожидалась ошибка подтверждения, got %q", api.lastText())
	}
}

// --- /stats ---

func TestCmdStats_Success(t *testing.T) {
	sites := &mockSites{
		statsFn: func(ctx context.Context, userID uuid.UUID) (domain.SiteStats, error) {
			return domain.SiteStats{TotalSites: 2, UpSites: 2}, nil
		},
	}
	h, api := newTestHandler(registeredUser(testTelegramID), sites)

	h.HandleMessage(context.Background(), cmdMessage(testTelegramID, testChatID, "/stats"))

	if !strings.Contains(api.lastText(), "Статистика") {
		t.Errorf("ожидалась статистика, got %q", api.lastText())
	}
}

func TestCmdStats_Error(t *testing.T) {
	sites := &mockSites{
		statsFn: func(ctx context.Context, userID uuid.UUID) (domain.SiteStats, error) {
			return domain.SiteStats{}, errors.New("boom")
		},
	}
	h, api := newTestHandler(registeredUser(testTelegramID), sites)

	h.HandleMessage(context.Background(), cmdMessage(testTelegramID, testChatID, "/stats"))

	if !strings.Contains(api.lastText(), "Не удалось получить статистику") {
		t.Errorf("ожидалась ошибка, got %q", api.lastText())
	}
}

// reply не паникует при ошибке отправки (только логирует).
func TestReply_SendErrorDoesNotPanic(t *testing.T) {
	api := &mockBotAPI{sendErr: errors.New("telegram down")}
	h := NewHandler(api, registeredUser(testTelegramID), &mockSites{})

	// Должно просто залогировать ошибку.
	h.HandleMessage(context.Background(), cmdMessage(testTelegramID, testChatID, "/start"))

	if api.count() != 0 {
		t.Errorf("при ошибке отправки сообщения не должно быть в списке успешных, got %d", api.count())
	}
}
