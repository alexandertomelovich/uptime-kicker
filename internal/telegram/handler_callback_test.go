// Тесты обработки нажатий инлайн-кнопок (callback.go).
package telegram

import (
	"context"
	"errors"
	"strings"
	"testing"

	"health_checker/internal/domain"

	"github.com/google/uuid"
)

// ownedSite — сайт, принадлежащий пользователю registeredUser(testTelegramID).
func ownedSite(id uuid.UUID, userID uuid.UUID, isActive bool) domain.Site {
	return domain.Site{ID: id, UserID: userID, Name: "Site", Url: "https://s", IsActive: isActive}
}

// nil callback и callback без From игнорируются.
func TestHandleCallback_NilSafe(t *testing.T) {
	h, api := newTestHandler(registeredUser(testTelegramID), &mockSites{})

	// nil callback.
	h.HandleCallback(context.Background(), nil)

	if api.requestCount() != 0 || api.count() != 0 {
		t.Errorf("на nil-callback не должно быть активности")
	}
}

// Битый callback data -> подтверждение нажатия отправлено, но действие не выполнено.
func TestHandleCallback_MalformedData(t *testing.T) {
	h, api := newTestHandler(registeredUser(testTelegramID), &mockSites{})

	h.HandleCallback(context.Background(), callbackQuery(testTelegramID, testChatID, "no-separator"))

	if api.requestCount() != 1 {
		t.Errorf("нажатие должно быть подтверждено, requestCount = %d", api.requestCount())
	}
	if api.count() != 0 {
		t.Errorf("при битом callback data не должно быть ответов, got %d", api.count())
	}
}

// Пользователь не найден -> сообщение о регистрации, действие не выполняется.
func TestHandleCallback_UserNotFound(t *testing.T) {
	users := &mockUsers{err: domain.ErrNotFound}
	h, api := newTestHandler(users, &mockSites{})

	h.HandleCallback(context.Background(), callbackQuery(testTelegramID, testChatID, "status:"+uuid.New().String()))

	if !strings.Contains(api.lastText(), "Аккаунт не найден") {
		t.Errorf("ожидалось сообщение об отсутствии аккаунта, got %q", api.lastText())
	}
}

// Невалидный UUID сайта в callback -> сообщение про некорректный ID.
func TestHandleCallback_InvalidSiteID(t *testing.T) {
	h, api := newTestHandler(registeredUser(testTelegramID), &mockSites{})

	h.HandleCallback(context.Background(), callbackQuery(testTelegramID, testChatID, "status:not-a-uuid"))

	if !strings.Contains(api.lastText(), "Некорректный ID") {
		t.Errorf("ожидалась ошибка про ID, got %q", api.lastText())
	}
}

// Сайт не найден -> соответствующее сообщение.
func TestHandleCallback_SiteNotFound(t *testing.T) {
	sites := &mockSites{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.Site, error) {
			return domain.Site{}, domain.ErrNotFound
		},
	}
	h, api := newTestHandler(registeredUser(testTelegramID), sites)

	h.HandleCallback(context.Background(), callbackQuery(testTelegramID, testChatID, "status:"+uuid.New().String()))

	if !strings.Contains(api.lastText(), "Сайт не найден") {
		t.Errorf("ожидалось 'Сайт не найден', got %q", api.lastText())
	}
}

// Чужой сайт -> отказ.
func TestHandleCallback_NotOwned(t *testing.T) {
	sites := &mockSites{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.Site, error) {
			return ownedSite(id, uuid.New(), true), nil // чужой владелец
		},
	}
	h, api := newTestHandler(registeredUser(testTelegramID), sites)

	h.HandleCallback(context.Background(), callbackQuery(testTelegramID, testChatID, "status:"+uuid.New().String()))

	if !strings.Contains(api.lastText(), "не принадлежит") {
		t.Errorf("ожидалась ошибка принадлежности, got %q", api.lastText())
	}
}

// action=toggle -> переключение активности и обновлённая карточка.
func TestHandleCallback_Toggle(t *testing.T) {
	user := registeredUser(testTelegramID)
	siteID := uuid.New()
	sites := &mockSites{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.Site, error) {
			return ownedSite(id, user.user.ID, true), nil
		},
		updateFn: func(ctx context.Context, update domain.SiteUpdate) (domain.Site, error) {
			// toggle инвертирует IsActive: true -> false (пауза).
			return domain.Site{ID: update.ID, IsActive: *update.IsActive, Name: "Site"}, nil
		},
	}
	h, api := newTestHandler(user, sites)

	h.HandleCallback(context.Background(), callbackQuery(testTelegramID, testChatID, "toggle:"+siteID.String()))

	if sites.lastUpdate.IsActive == nil || *sites.lastUpdate.IsActive {
		t.Errorf("ожидалось IsActive=false (пауза), got %v", sites.lastUpdate.IsActive)
	}
	if !strings.Contains(api.lastText(), "поставлен на паузу") {
		t.Errorf("ожидался текст про паузу, got %q", api.lastText())
	}
}

// action=confirm -> удаление сайта.
func TestHandleCallback_Confirm(t *testing.T) {
	user := registeredUser(testTelegramID)
	siteID := uuid.New()
	sites := &mockSites{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.Site, error) {
			return ownedSite(id, user.user.ID, true), nil
		},
	}
	h, api := newTestHandler(user, sites)

	h.HandleCallback(context.Background(), callbackQuery(testTelegramID, testChatID, "confirm:"+siteID.String()))

	if sites.lastDelete != siteID {
		t.Errorf("delete id = %v, want %v", sites.lastDelete, siteID)
	}
	if !strings.Contains(api.lastText(), "удалён") {
		t.Errorf("ожидалось сообщение об удалении, got %q", api.lastText())
	}
}

// action=confirm, ошибка удаления -> понятное сообщение.
func TestHandleCallback_ConfirmDeleteError(t *testing.T) {
	user := registeredUser(testTelegramID)
	siteID := uuid.New()
	sites := &mockSites{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.Site, error) {
			return ownedSite(id, user.user.ID, true), nil
		},
		deleteFn: func(ctx context.Context, id, userID uuid.UUID) error {
			return errors.New("boom")
		},
	}
	h, api := newTestHandler(user, sites)

	h.HandleCallback(context.Background(), callbackQuery(testTelegramID, testChatID, "confirm:"+siteID.String()))

	if !strings.Contains(api.lastText(), "Не удалось удалить") {
		t.Errorf("ожидалась ошибка удаления, got %q", api.lastText())
	}
}

// action=remove -> запрос подтверждения удаления.
func TestHandleCallback_Remove(t *testing.T) {
	user := registeredUser(testTelegramID)
	siteID := uuid.New()
	sites := &mockSites{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.Site, error) {
			return ownedSite(id, user.user.ID, true), nil
		},
	}
	h, api := newTestHandler(user, sites)

	h.HandleCallback(context.Background(), callbackQuery(testTelegramID, testChatID, "remove:"+siteID.String()))

	if !strings.Contains(api.lastText(), "Удалить сайт") {
		t.Errorf("ожидался запрос подтверждения, got %q", api.lastText())
	}
}

// action=status -> детальная карточка.
func TestHandleCallback_Status(t *testing.T) {
	user := registeredUser(testTelegramID)
	siteID := uuid.New()
	sites := &mockSites{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.Site, error) {
			return ownedSite(id, user.user.ID, true), nil
		},
	}
	h, api := newTestHandler(user, sites)

	h.HandleCallback(context.Background(), callbackQuery(testTelegramID, testChatID, "status:"+siteID.String()))

	if !strings.Contains(api.lastText(), "Site") {
		t.Errorf("ожидалась карточка сайта, got %q", api.lastText())
	}
}

// Неизвестное действие -> ответ не отправляется, но нажатие подтверждается.
func TestHandleCallback_UnknownAction(t *testing.T) {
	user := registeredUser(testTelegramID)
	siteID := uuid.New()
	sites := &mockSites{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.Site, error) {
			return ownedSite(id, user.user.ID, true), nil
		},
	}
	h, api := newTestHandler(user, sites)

	h.HandleCallback(context.Background(), callbackQuery(testTelegramID, testChatID, "weird:"+siteID.String()))

	if api.requestCount() != 1 {
		t.Errorf("нажатие должно быть подтверждено, requestCount = %d", api.requestCount())
	}
	if api.count() != 0 {
		t.Errorf("при неизвестном действии не должно быть ответов, got %d", api.count())
	}
}

// toggle: ошибка обновления -> сообщение об ошибке.
func TestHandleCallback_ToggleError(t *testing.T) {
	user := registeredUser(testTelegramID)
	siteID := uuid.New()
	sites := &mockSites{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.Site, error) {
			return ownedSite(id, user.user.ID, true), nil
		},
		updateFn: func(ctx context.Context, update domain.SiteUpdate) (domain.Site, error) {
			return domain.Site{}, errors.New("boom")
		},
	}
	h, api := newTestHandler(user, sites)

	h.HandleCallback(context.Background(), callbackQuery(testTelegramID, testChatID, "toggle:"+siteID.String()))

	if !strings.Contains(api.lastText(), "Не удалось изменить") {
		t.Errorf("ожидалась ошибка изменения, got %q", api.lastText())
	}
}
