// Тесты для UserService.
//
// UserService — это основная бизнес-логика работы с пользователями: регистрация,
// вход, управление профилем и правами. Внешние зависимости (репозиторий, JWT,
// нотификатор и хранилище refresh-сессий) подменяются моками/стабами.
//
// Особенности:
//   - часть методов требует аутентификации: Claims берутся из контекста через
//     auth.GetUserFromContext, поэтому в тестах контекст наполняется вручную
//     хелпером ctxWithUser;
//   - пароли хешируются bcrypt, поэтому проверяем не конкретный хеш, а то, что
//     он непустой и отличается от открытого пароля (+ успешный CompareHash);
//   - ошибки проверяются через errors.Is — это подтверждает оборачивание %w.
package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"health_checker/internal/auth"
	"health_checker/internal/domain"
	"health_checker/internal/notifier"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// --- Стабы зависимостей UserService ---

// capturingSender — notifier.Sender, который запоминает отправленные сообщения.
// Регистрация шлёт welcome-сообщение в отдельной горутине, поэтому доступ к
// срезу защищён мьютексом. Также умеет имитировать ошибку отправки.
type capturingSender struct {
	mu       sync.Mutex
	messages []notifier.Message
	err      error
}

func (s *capturingSender) Send(ctx context.Context, message notifier.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, message)
	return s.err
}

func (s *capturingSender) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.messages)
}

// mockTokenStore реализует auth.TokenStore и фиксирует вызовы отзыва сессий.
// В тестах важно лишь то, сколько раз был вызван DeleteAllForUser (инвалидация
// всех сессий при смене пароля/роли/удалении аккаунта).
type mockTokenStore struct {
	mu            sync.Mutex
	deletedAllFor []uuid.UUID
	deleteAllErr  error
}

func (m *mockTokenStore) Save(ctx context.Context, jti, userID uuid.UUID, expiresAt time.Time) error {
	return nil
}

func (m *mockTokenStore) Take(ctx context.Context, jti uuid.UUID) (uuid.UUID, error) {
	return uuid.Nil, auth.ErrTokenRevoked
}

func (m *mockTokenStore) Delete(ctx context.Context, jti uuid.UUID) error { return nil }

func (m *mockTokenStore) DeleteAllForUser(ctx context.Context, userID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deletedAllFor = append(m.deletedAllFor, userID)
	return m.deleteAllErr
}

func (m *mockTokenStore) deleteAllCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.deletedAllFor)
}

// --- Хелперы ---

// testJWTManager создаёт JWTManager с короткими TTL и без хранилища сессий —
// этого достаточно для генерации/валидации токенов в Login.
func testJWTManager() *auth.JWTManager {
	return auth.NewJWTManager("access-secret", "refresh-secret", time.Minute, time.Hour)
}

// ctxWithUser возвращает контекст с уже положенными Claims — имитирует то, что
// делает AuthMiddleware после проверки access-токена.
func ctxWithUser(userID uuid.UUID, role domain.Role) context.Context {
	claims := &auth.Claims{UserID: userID, Role: role}
	return context.WithValue(context.Background(), auth.UserContextKey, claims)
}

// validPassword удовлетворяет DefaultPolicy (длина, цифра, регистр, спецсимвол).
const validPassword = "Passw0rd!"

// hashOf возвращает bcrypt-хеш пароля (для подготовки «сохранённого» пользователя).
func hashOf(t *testing.T, password string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("bcrypt.GenerateFromPassword: %v", err)
	}
	return string(h)
}

func newTestUserService(repo UserRepository, notif notifier.Sender, tokens auth.TokenStore) *UserService {
	if notif == nil {
		notif = &capturingSender{}
	}
	return NewUserService(repo, notif, testJWTManager(), tokens)
}

// --- Register ---

// Успешная регистрация: пользователю ставится роль user, email нормализуется
// в нижний регистр, пароль сохраняется в виде bcrypt-хеша (не в открытом виде),
// а репозиторию отдаётся именно созданный пользователь.
func TestUserService_Register_Success(t *testing.T) {
	repo := &mockUserRepo{}
	sender := &capturingSender{}
	svc := newTestUserService(repo, sender, nil)

	user, err := svc.Register(context.Background(), RegisterRequest{
		Name:       "Alice",
		Email:      "Alice@Example.COM",
		TelegramID: 42,
		Password:   validPassword,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if repo.lastCreated.Email != "alice@example.com" {
		t.Errorf("email = %q, want alice@example.com", repo.lastCreated.Email)
	}
	if repo.lastCreated.Role != domain.RoleUser {
		t.Errorf("role = %q, want user", repo.lastCreated.Role)
	}
	if repo.lastCreated.PasswordHash == validPassword {
		t.Error("пароль не должен храниться в открытом виде")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(repo.lastCreated.PasswordHash), []byte(validPassword)); err != nil {
		t.Errorf("сохранённый хеш не соответствует паролю: %v", err)
	}
	if user.ID == uuid.Nil {
		t.Error("Register должен вернуть пользователя с присвоенным ID")
	}
}

// Зарегистрированный email должен быть недоступен повторно.
func TestUserService_Register_EmailAlreadyExists(t *testing.T) {
	repo := &mockUserRepo{
		getByEmailFn: func(ctx context.Context, email string) (domain.User, error) {
			return domain.User{ID: uuid.New(), Email: email}, nil
		},
	}
	svc := newTestUserService(repo, nil, nil)

	_, err := svc.Register(context.Background(), RegisterRequest{
		Email: "dup@example.com", TelegramID: 1, Password: validPassword,
	})
	if !errors.Is(err, domain.ErrEmailAlreadyExists) {
		t.Fatalf("err = %v, want ErrEmailAlreadyExists", err)
	}
}

// Ошибка проверки email (не NotFound) оборачивается и прерывает регистрацию.
func TestUserService_Register_EmailCheckError(t *testing.T) {
	repo := &mockUserRepo{
		getByEmailFn: func(ctx context.Context, email string) (domain.User, error) {
			return domain.User{}, errBoom
		},
	}
	svc := newTestUserService(repo, nil, nil)

	_, err := svc.Register(context.Background(), RegisterRequest{
		Email: "x@example.com", TelegramID: 1, Password: validPassword,
	})
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want wrapped errBoom", err)
	}
}

// TelegramID не должен дублироваться.
func TestUserService_Register_TelegramAlreadyExists(t *testing.T) {
	repo := &mockUserRepo{
		getByTelegramIDFn: func(ctx context.Context, telegramID int64) (domain.User, error) {
			return domain.User{ID: uuid.New(), TelegramID: telegramID}, nil
		},
	}
	svc := newTestUserService(repo, nil, nil)

	_, err := svc.Register(context.Background(), RegisterRequest{
		Email: "x@example.com", TelegramID: 7, Password: validPassword,
	})
	if !errors.Is(err, domain.ErrTelegramAlreadyExists) {
		t.Fatalf("err = %v, want ErrTelegramAlreadyExists", err)
	}
}

// Ошибка проверки telegram (не NotFound) оборачивается.
func TestUserService_Register_TelegramCheckError(t *testing.T) {
	repo := &mockUserRepo{
		getByTelegramIDFn: func(ctx context.Context, telegramID int64) (domain.User, error) {
			return domain.User{}, errBoom
		},
	}
	svc := newTestUserService(repo, nil, nil)

	_, err := svc.Register(context.Background(), RegisterRequest{
		Email: "x@example.com", TelegramID: 1, Password: validPassword,
	})
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want wrapped errBoom", err)
	}
}

// Пароль, не проходящий политику, отклоняется с ErrPasswordPolicy.
func TestUserService_Register_WeakPassword(t *testing.T) {
	svc := newTestUserService(&mockUserRepo{}, nil, nil)

	_, err := svc.Register(context.Background(), RegisterRequest{
		Email: "x@example.com", TelegramID: 1, Password: "short",
	})
	if !errors.Is(err, domain.ErrPasswordPolicy) {
		t.Fatalf("err = %v, want ErrPasswordPolicy", err)
	}
}

// Ошибка репозитория при создании оборачивается.
func TestUserService_Register_CreateError(t *testing.T) {
	repo := &mockUserRepo{
		createFn: func(ctx context.Context, user domain.User) (uuid.UUID, error) {
			return uuid.Nil, errBoom
		},
	}
	svc := newTestUserService(repo, nil, nil)

	_, err := svc.Register(context.Background(), RegisterRequest{
		Email: "x@example.com", TelegramID: 1, Password: validPassword,
	})
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want wrapped errBoom", err)
	}
}

// Успешная регистрация асинхронно шлёт welcome-сообщение в Telegram.
func TestUserService_Register_SendsWelcome(t *testing.T) {
	repo := &mockUserRepo{}
	sender := &capturingSender{}
	svc := newTestUserService(repo, sender, nil)

	if _, err := svc.Register(context.Background(), RegisterRequest{
		Name: "Bob", Email: "bob@example.com", TelegramID: 99, Password: validPassword,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// sendWelcome выполняется в отдельной горутине — подождём её завершения.
	deadline := time.Now().Add(2 * time.Second)
	for sender.count() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if sender.count() == 0 {
		t.Fatal("ожидалось welcome-сообщение, но Send не был вызван")
	}
	if !strings.Contains(sender.messages[0].Text, "Bob") {
		t.Errorf("welcome-сообщение не содержит имя: %q", sender.messages[0].Text)
	}
}

// Имя пользователя подставляется в welcome-сообщение с ParseMode=HTML, поэтому
// должно быть экранировано: иначе регистрация с именем "<b>" ломает разметку.
func TestUserService_SendWelcome_EscapesName(t *testing.T) {
	sender := &capturingSender{}
	svc := newTestUserService(&mockUserRepo{}, sender, nil)

	svc.sendWelcome(&domain.User{ID: uuid.New(), TelegramID: 99, Name: `<b>hack</b>`})

	if sender.count() != 1 {
		t.Fatalf("сообщений = %d, want 1", sender.count())
	}
	msg := sender.messages[0]
	if msg.ParseMode != notifier.ParseModeHTML {
		t.Errorf("ParseMode = %q, want %q", msg.ParseMode, notifier.ParseModeHTML)
	}
	if strings.Contains(msg.Text, "<b>") {
		t.Errorf("имя не экранировано: %q", msg.Text)
	}
	if !strings.Contains(msg.Text, "&lt;b&gt;hack&lt;/b&gt;") {
		t.Errorf("ожидалось экранированное имя: %q", msg.Text)
	}
	// Вместе с именем проверяем и статическую подсказку "/add <url>": в HTML она
	// тоже должна быть записана сущностями.
	if strings.ContainsAny(msg.Text, "<>") {
		t.Errorf("в сообщении остались сырые угловые скобки: %q", msg.Text)
	}
}

// --- Login ---

// Успешный вход: email приводится к нижнему регистру, пароль сверяется с хешем,
// и возвращается непустая пара токенов.
func TestUserService_Login_Success(t *testing.T) {
	stored := domain.User{
		ID:           uuid.New(),
		Email:        "alice@example.com",
		Role:         domain.RoleUser,
		TelegramID:   5,
		PasswordHash: hashOf(t, validPassword),
	}
	repo := &mockUserRepo{
		getByEmailFn: func(ctx context.Context, email string) (domain.User, error) {
			if email != "alice@example.com" {
				t.Errorf("email = %q, want normalised lowercase", email)
			}
			return stored, nil
		},
	}
	svc := newTestUserService(repo, nil, nil)

	pair, err := svc.Login(context.Background(), "Alice@Example.com", validPassword)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatalf("ожидались непустые токены, got %+v", pair)
	}
	if pair.TokenType != "Bearer" {
		t.Errorf("token_type = %q, want Bearer", pair.TokenType)
	}
}

// Несуществующий пользователь даёт ErrInvalidCredentials (а не ErrNotFound,
// чтобы не раскрывать наличие аккаунта).
func TestUserService_Login_UserNotFound(t *testing.T) {
	repo := &mockUserRepo{
		getByEmailFn: func(ctx context.Context, email string) (domain.User, error) {
			return domain.User{}, domain.ErrNotFound
		},
	}
	svc := newTestUserService(repo, nil, nil)

	_, err := svc.Login(context.Background(), "nobody@example.com", validPassword)
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("err = %v, want ErrInvalidCredentials", err)
	}
}

// Неверный пароль тоже даёт ErrInvalidCredentials.
func TestUserService_Login_WrongPassword(t *testing.T) {
	repo := &mockUserRepo{
		getByEmailFn: func(ctx context.Context, email string) (domain.User, error) {
			return domain.User{ID: uuid.New(), PasswordHash: hashOf(t, validPassword)}, nil
		},
	}
	svc := newTestUserService(repo, nil, nil)

	_, err := svc.Login(context.Background(), "alice@example.com", "wrong-Passw0rd!")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("err = %v, want ErrInvalidCredentials", err)
	}
}

// Сбой БД при поиске пользователя — внутренняя ошибка, а не «неверные
// учётные данные» (клиент не должен принимать падение БД за неверный пароль).
func TestUserService_Login_RepoErrorWrapped(t *testing.T) {
	repo := &mockUserRepo{
		getByEmailFn: func(ctx context.Context, email string) (domain.User, error) {
			return domain.User{}, errBoom
		},
	}
	svc := newTestUserService(repo, nil, nil)

	_, err := svc.Login(context.Background(), "alice@example.com", validPassword)
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want wrapped errBoom", err)
	}
}

// telegram_id <= 0 отклоняется: на него уходят уведомления, а 0 как
// уникальное значение делало бы второго пользователя без Telegram
// невозможным.
func TestUserService_Register_InvalidTelegramID(t *testing.T) {
	svc := newTestUserService(&mockUserRepo{}, nil, nil)

	_, err := svc.Register(context.Background(), RegisterRequest{
		Email: "x@example.com", TelegramID: 0, Password: validPassword,
	})
	if !errors.Is(err, domain.ErrInvalidTelegramID) {
		t.Fatalf("err = %v, want ErrInvalidTelegramID", err)
	}
}

// --- Delete ---

// Админ может удалить пользователя; после удаления отзываются все его сессии.
func TestUserService_Delete_AdminSuccess(t *testing.T) {
	targetID := uuid.New()
	deleted := false
	store := &mockTokenStore{}
	repo := &mockUserRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.User, error) {
			return domain.User{ID: targetID}, nil
		},
		deleteFn: func(ctx context.Context, id uuid.UUID) error {
			deleted = true
			return nil
		},
	}
	svc := newTestUserService(repo, nil, store)

	ctx := ctxWithUser(uuid.New(), domain.RoleAdmin)
	if err := svc.Delete(ctx, targetID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !deleted {
		t.Error("ожидался вызов repo.Delete")
	}
	if store.deleteAllCount() != 1 {
		t.Errorf("DeleteAllForUser вызван %d раз, want 1", store.deleteAllCount())
	}
}

// Не-админ не может удалять пользователей.
func TestUserService_Delete_AccessDenied(t *testing.T) {
	svc := newTestUserService(&mockUserRepo{}, nil, nil)

	ctx := ctxWithUser(uuid.New(), domain.RoleUser)
	err := svc.Delete(ctx, uuid.New())
	if !errors.Is(err, domain.ErrAccessDenied) {
		t.Fatalf("err = %v, want ErrAccessDenied", err)
	}
}

// Нет Claims в контексте -> ErrUnauthorized.
func TestUserService_Delete_Unauthorized(t *testing.T) {
	svc := newTestUserService(&mockUserRepo{}, nil, nil)

	err := svc.Delete(context.Background(), uuid.New())
	if !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
}

// Целевой пользователь не найден -> ErrNotFound (до вызова Delete).
func TestUserService_Delete_TargetNotFound(t *testing.T) {
	repo := &mockUserRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.User, error) {
			return domain.User{}, domain.ErrNotFound
		},
	}
	svc := newTestUserService(repo, nil, nil)

	ctx := ctxWithUser(uuid.New(), domain.RoleAdmin)
	err := svc.Delete(ctx, uuid.New())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// Ошибка токен-стора при отзыве сессий оборачивается.
func TestUserService_Delete_RevokeSessionsError(t *testing.T) {
	store := &mockTokenStore{deleteAllErr: errBoom}
	repo := &mockUserRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.User, error) {
			return domain.User{ID: id}, nil
		},
	}
	svc := newTestUserService(repo, nil, store)

	ctx := ctxWithUser(uuid.New(), domain.RoleAdmin)
	err := svc.Delete(ctx, uuid.New())
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want wrapped errBoom", err)
	}
}

// --- GetAll ---

// Админ получает список пользователей из репозитория.
func TestUserService_GetAll_AdminSuccess(t *testing.T) {
	want := []domain.User{{ID: uuid.New()}, {ID: uuid.New()}}
	repo := &mockUserRepo{
		getAllFn: func(ctx context.Context) ([]domain.User, error) {
			return want, nil
		},
	}
	svc := newTestUserService(repo, nil, nil)

	ctx := ctxWithUser(uuid.New(), domain.RoleAdmin)
	got, err := svc.GetAll(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
}

// Не-админ получает ErrAccessDenied.
func TestUserService_GetAll_AccessDenied(t *testing.T) {
	svc := newTestUserService(&mockUserRepo{}, nil, nil)

	ctx := ctxWithUser(uuid.New(), domain.RoleUser)
	_, err := svc.GetAll(ctx)
	if !errors.Is(err, domain.ErrAccessDenied) {
		t.Fatalf("err = %v, want ErrAccessDenied", err)
	}
}

// Ошибка репозитория оборачивается.
func TestUserService_GetAll_RepoError(t *testing.T) {
	repo := &mockUserRepo{
		getAllFn: func(ctx context.Context) ([]domain.User, error) {
			return nil, errBoom
		},
	}
	svc := newTestUserService(repo, nil, nil)

	ctx := ctxWithUser(uuid.New(), domain.RoleAdmin)
	_, err := svc.GetAll(ctx)
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want wrapped errBoom", err)
	}
}

// --- Read-методы (доступ только админам) ---

// Все read-методы требуют роль администратора: обычный пользователь получает
// ErrAccessDenied, не доходя до репозитория.
func TestUserService_Getters_AccessDenied(t *testing.T) {
	svc := newTestUserService(&mockUserRepo{}, nil, nil)
	ctx := ctxWithUser(uuid.New(), domain.RoleUser)

	if _, err := svc.GetByEmail(ctx, "a@example.com"); !errors.Is(err, domain.ErrAccessDenied) {
		t.Errorf("GetByEmail: err = %v, want ErrAccessDenied", err)
	}
	if _, err := svc.GetByID(ctx, uuid.New()); !errors.Is(err, domain.ErrAccessDenied) {
		t.Errorf("GetByID: err = %v, want ErrAccessDenied", err)
	}
	if _, err := svc.GetByTelegramID(ctx, 1); !errors.Is(err, domain.ErrAccessDenied) {
		t.Errorf("GetByTelegramID: err = %v, want ErrAccessDenied", err)
	}
	if _, err := svc.GetByUsername(ctx, "x"); !errors.Is(err, domain.ErrAccessDenied) {
		t.Errorf("GetByUsername: err = %v, want ErrAccessDenied", err)
	}
}

func TestUserService_GetByEmail_AdminSuccess(t *testing.T) {
	want := domain.User{ID: uuid.New(), Email: "a@example.com"}
	repo := &mockUserRepo{
		getByEmailFn: func(ctx context.Context, email string) (domain.User, error) {
			if email != "a@example.com" {
				t.Errorf("email = %q", email)
			}
			return want, nil
		},
	}
	svc := newTestUserService(repo, nil, nil)

	ctx := ctxWithUser(uuid.New(), domain.RoleAdmin)
	got, err := svc.GetByEmail(ctx, "a@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("ID = %v, want %v", got.ID, want.ID)
	}
}

func TestUserService_GetByEmail_RepoError(t *testing.T) {
	repo := &mockUserRepo{
		getByEmailFn: func(ctx context.Context, email string) (domain.User, error) {
			return domain.User{}, errBoom
		},
	}
	svc := newTestUserService(repo, nil, nil)

	ctx := ctxWithUser(uuid.New(), domain.RoleAdmin)
	_, err := svc.GetByEmail(ctx, "a@example.com")
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want wrapped errBoom", err)
	}
}

// GetByID сохраняет ErrNotFound как есть (без оборачивания), чтобы handler мог
// вернуть 404.
func TestUserService_GetByID_AdminSuccess(t *testing.T) {
	want := domain.User{ID: uuid.New()}
	repo := &mockUserRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.User, error) {
			return want, nil
		},
	}
	svc := newTestUserService(repo, nil, nil)

	ctx := ctxWithUser(uuid.New(), domain.RoleAdmin)
	got, err := svc.GetByID(ctx, want.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("ID = %v, want %v", got.ID, want.ID)
	}
}

func TestUserService_GetByID_NotFoundPreserved(t *testing.T) {
	repo := &mockUserRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.User, error) {
			return domain.User{}, domain.ErrNotFound
		},
	}
	svc := newTestUserService(repo, nil, nil)

	ctx := ctxWithUser(uuid.New(), domain.RoleAdmin)
	_, err := svc.GetByID(ctx, uuid.New())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestUserService_GetByID_RepoError(t *testing.T) {
	repo := &mockUserRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.User, error) {
			return domain.User{}, errBoom
		},
	}
	svc := newTestUserService(repo, nil, nil)

	ctx := ctxWithUser(uuid.New(), domain.RoleAdmin)
	_, err := svc.GetByID(ctx, uuid.New())
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want wrapped errBoom", err)
	}
}

func TestUserService_GetByTelegramID_AdminSuccess(t *testing.T) {
	want := domain.User{ID: uuid.New(), TelegramID: 123}
	repo := &mockUserRepo{
		getByTelegramIDFn: func(ctx context.Context, telegramID int64) (domain.User, error) {
			return want, nil
		},
	}
	svc := newTestUserService(repo, nil, nil)

	ctx := ctxWithUser(uuid.New(), domain.RoleAdmin)
	got, err := svc.GetByTelegramID(ctx, 123)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.TelegramID != 123 {
		t.Errorf("TelegramID = %d, want 123", got.TelegramID)
	}
}

func TestUserService_GetByUsername_AdminSuccess(t *testing.T) {
	want := []domain.User{{ID: uuid.New(), Name: "Ann"}, {ID: uuid.New(), Name: "Anna"}}
	repo := &mockUserRepo{
		getByUsernameFn: func(ctx context.Context, username string) ([]domain.User, error) {
			if username != "Ann" {
				t.Errorf("username = %q", username)
			}
			return want, nil
		},
	}
	svc := newTestUserService(repo, nil, nil)

	ctx := ctxWithUser(uuid.New(), domain.RoleAdmin)
	got, err := svc.GetByUsername(ctx, "Ann")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
}

// --- Update ---

// Пользователь может обновить сам себя: email нормализуется, обновление уходит
// в репозиторий, а смена только имени не отзывает сессии.
func TestUserService_Update_SelfName(t *testing.T) {
	userID := uuid.New()
	name := "New Name"
	repo := &mockUserRepo{}
	store := &mockTokenStore{}
	svc := newTestUserService(repo, nil, store)

	ctx := ctxWithUser(userID, domain.RoleUser)
	if err := svc.Update(ctx, UpdateUserParams{ID: userID, Name: &name}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.lastUpdate.Name == nil || *repo.lastUpdate.Name != name {
		t.Errorf("update.Name = %v, want %q", repo.lastUpdate.Name, name)
	}
	if store.deleteAllCount() != 0 {
		t.Errorf("при смене только имени сессии отзываться не должны, got %d", store.deleteAllCount())
	}
}

// Обычный пользователь не может менять профиль другого пользователя.
func TestUserService_Update_OtherUserForbidden(t *testing.T) {
	svc := newTestUserService(&mockUserRepo{}, nil, nil)

	ctx := ctxWithUser(uuid.New(), domain.RoleUser)
	err := svc.Update(ctx, UpdateUserParams{ID: uuid.New()})
	if !errors.Is(err, domain.ErrAccessDenied) {
		t.Fatalf("err = %v, want ErrAccessDenied", err)
	}
}

// Нет Claims -> ErrUnauthorized.
func TestUserService_Update_Unauthorized(t *testing.T) {
	svc := newTestUserService(&mockUserRepo{}, nil, nil)

	err := svc.Update(context.Background(), UpdateUserParams{ID: uuid.New()})
	if !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
}

// Менять роль может только админ.
func TestUserService_Update_RoleChangeForbidden(t *testing.T) {
	userID := uuid.New()
	role := domain.RoleAdmin
	svc := newTestUserService(&mockUserRepo{}, nil, nil)

	ctx := ctxWithUser(userID, domain.RoleUser)
	err := svc.Update(ctx, UpdateUserParams{ID: userID, Role: &role})
	if !errors.Is(err, domain.ErrRoleChangeForbidden) {
		t.Fatalf("err = %v, want ErrRoleChangeForbidden", err)
	}
}

// Роль должна быть валидной.
func TestUserService_Update_InvalidRole(t *testing.T) {
	role := domain.Role("superuser")
	svc := newTestUserService(&mockUserRepo{}, nil, nil)

	ctx := ctxWithUser(uuid.New(), domain.RoleAdmin)
	err := svc.Update(ctx, UpdateUserParams{ID: uuid.New(), Role: &role})
	if !errors.Is(err, domain.ErrInvalidRole) {
		t.Fatalf("err = %v, want ErrInvalidRole", err)
	}
}

// Email нормализуется в нижний регистр перед записью.
func TestUserService_Update_EmailNormalisedAndUnique(t *testing.T) {
	userID := uuid.New()
	email := "New@Example.COM"
	repo := &mockUserRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.User, error) {
			return domain.User{ID: id, Email: "old@example.com"}, nil
		},
	}
	svc := newTestUserService(repo, nil, nil)

	ctx := ctxWithUser(userID, domain.RoleUser)
	if err := svc.Update(ctx, UpdateUserParams{ID: userID, Email: &email}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.lastUpdate.Email == nil || *repo.lastUpdate.Email != "new@example.com" {
		t.Errorf("update.Email = %v, want new@example.com", repo.lastUpdate.Email)
	}
}

// Если новый email занят другим пользователем — ErrEmailAlreadyExists.
func TestUserService_Update_EmailTaken(t *testing.T) {
	userID := uuid.New()
	other := uuid.New()
	email := "taken@example.com"
	repo := &mockUserRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.User, error) {
			return domain.User{ID: id, Email: "old@example.com"}, nil
		},
		getByEmailFn: func(ctx context.Context, e string) (domain.User, error) {
			return domain.User{ID: other, Email: e}, nil
		},
	}
	svc := newTestUserService(repo, nil, nil)

	ctx := ctxWithUser(userID, domain.RoleUser)
	err := svc.Update(ctx, UpdateUserParams{ID: userID, Email: &email})
	if !errors.Is(err, domain.ErrEmailAlreadyExists) {
		t.Fatalf("err = %v, want ErrEmailAlreadyExists", err)
	}
}

// Слабый пароль при обновлении отклоняется.
func TestUserService_Update_WeakPassword(t *testing.T) {
	userID := uuid.New()
	pw := "weak"
	svc := newTestUserService(&mockUserRepo{}, nil, nil)

	ctx := ctxWithUser(userID, domain.RoleUser)
	err := svc.Update(ctx, UpdateUserParams{ID: userID, Password: &pw})
	if !errors.Is(err, domain.ErrPasswordPolicy) {
		t.Fatalf("err = %v, want ErrPasswordPolicy", err)
	}
}

// Смена пароля хеширует его и отзывает все refresh-сессии пользователя.
func TestUserService_Update_PasswordRevokesSessions(t *testing.T) {
	userID := uuid.New()
	pw := validPassword
	repo := &mockUserRepo{}
	store := &mockTokenStore{}
	svc := newTestUserService(repo, nil, store)

	ctx := ctxWithUser(userID, domain.RoleUser)
	if err := svc.Update(ctx, UpdateUserParams{ID: userID, Password: &pw}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.lastUpdate.PasswordHash == nil {
		t.Fatal("PasswordHash не установлен")
	}
	if *repo.lastUpdate.PasswordHash == pw {
		t.Error("пароль не должен сохраняться в открытом виде")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*repo.lastUpdate.PasswordHash), []byte(pw)); err != nil {
		t.Errorf("хеш не соответствует паролю: %v", err)
	}
	if store.deleteAllCount() != 1 {
		t.Errorf("DeleteAllForUser вызван %d раз, want 1", store.deleteAllCount())
	}
}

// Смена роли админом также отзывает сессии (иначе клиент сохранит старую роль).
func TestUserService_Update_RoleRevokesSessions(t *testing.T) {
	targetID := uuid.New()
	role := domain.RoleUser
	store := &mockTokenStore{}
	repo := &mockUserRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.User, error) {
			return domain.User{ID: targetID, Role: domain.RoleAdmin}, nil
		},
	}
	svc := newTestUserService(repo, nil, store)

	ctx := ctxWithUser(uuid.New(), domain.RoleAdmin)
	if err := svc.Update(ctx, UpdateUserParams{ID: targetID, Role: &role}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store.deleteAllCount() != 1 {
		t.Errorf("DeleteAllForUser вызван %d раз, want 1", store.deleteAllCount())
	}
}

// Ошибка репозитория при обновлении оборачивается.
func TestUserService_Update_RepoError(t *testing.T) {
	userID := uuid.New()
	name := "X"
	repo := &mockUserRepo{
		updateFn: func(ctx context.Context, update domain.UserUpdate) error {
			return errBoom
		},
	}
	svc := newTestUserService(repo, nil, nil)

	ctx := ctxWithUser(userID, domain.RoleUser)
	err := svc.Update(ctx, UpdateUserParams{ID: userID, Name: &name})
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want wrapped errBoom", err)
	}
}

// --- RefreshToken / Logout ---

// RefreshToken делегирует работу JWTManager: с валидным refresh-токеном он
// возвращает новую пару токенов.
func TestUserService_RefreshToken_Success(t *testing.T) {
	repo := &mockUserRepo{
		getByEmailFn: func(ctx context.Context, email string) (domain.User, error) {
			return domain.User{ID: uuid.New(), Email: email, Role: domain.RoleUser, PasswordHash: hashOf(t, validPassword)}, nil
		},
	}
	svc := newTestUserService(repo, nil, nil)

	// Сначала логинимся, чтобы получить настоящий refresh-токен.
	pair, err := svc.Login(context.Background(), "alice@example.com", validPassword)
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	newPair, err := svc.RefreshToken(context.Background(), pair.RefreshToken)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if newPair.AccessToken == "" || newPair.RefreshToken == "" {
		t.Fatalf("ожидались непустые токены, got %+v", newPair)
	}
}

// Невалидный refresh-токен -> ошибка (обёрнутая).
func TestUserService_RefreshToken_Invalid(t *testing.T) {
	svc := newTestUserService(&mockUserRepo{}, nil, nil)

	_, err := svc.RefreshToken(context.Background(), "not-a-token")
	if err == nil {
		t.Fatal("ожидалась ошибка для невалидного токена")
	}
}

// Logout отзывает сессию, связанную с refresh-токеном.
func TestUserService_Logout_Success(t *testing.T) {
	svc := newTestUserService(&mockUserRepo{}, nil, nil)

	pair, err := svc.jwtManager.GenerateTokenPair(context.Background(), uuid.New(), "a@example.com", domain.RoleUser, 0)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	if err := svc.Logout(context.Background(), pair.RefreshToken); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Невалидный refresh-токен при Logout -> обёрнутая ошибка.
func TestUserService_Logout_Invalid(t *testing.T) {
	svc := newTestUserService(&mockUserRepo{}, nil, nil)

	err := svc.Logout(context.Background(), "garbage")
	if err == nil {
		t.Fatal("ожидалась ошибка для невалидного токена")
	}
}
