package httphandler

import (
	"context"
	"health_checker/internal/auth"
	"health_checker/internal/domain"
	"health_checker/internal/service"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// UserService описывает методы бизнес-логики, необходимые HTTP-слою.
// Интерфейс объявлен на стороне потребителя, чтобы упростить подмену в тестах.
type UserService interface {
	Register(ctx context.Context, req service.RegisterRequest) (domain.User, error)
	Login(ctx context.Context, email, password string) (*auth.TokenPair, error)
	RefreshToken(ctx context.Context, refreshToken string) (*auth.TokenPair, error)
	GetAll(ctx context.Context) ([]domain.User, error)
	GetByID(ctx context.Context, id uuid.UUID) (domain.User, error)
	Update(ctx context.Context, params service.UpdateUserParams) error
	Delete(ctx context.Context, id uuid.UUID) error
}

// UserHandler обслуживает HTTP-запросы, связанные с пользователями и аутентификацией.
type UserHandler struct {
	svc UserService
}

func NewUserHandler(svc UserService) *UserHandler {
	if svc == nil {
		panic("UserHandler: svc is nil")
	}
	return &UserHandler{svc: svc}
}

// Routes регистрирует маршруты handler-а на переданном роутере.
// authMiddleware — middleware аутентификации (например, JWTManager.AuthMiddleware),
// применяемый к защищённым маршрутам.
func (h *UserHandler) Routes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	// Публичные маршруты аутентификации.
	r.Post("/auth/register", h.Register)
	r.Post("/auth/login", h.Login)
	r.Post("/auth/refresh", h.Refresh)

	// Защищённые маршруты пользователей.
	r.Group(func(r chi.Router) {
		r.Use(authMiddleware)
		r.Get("/users", h.GetAll)
		r.Get("/users/{id}", h.GetByID)
		r.Patch("/users/{id}", h.Update)
		r.Delete("/users/{id}", h.Delete)
	})
}

// Register создаёт нового пользователя. POST /auth/register
type registerRequest struct {
	Name       string `json:"name"`
	Email      string `json:"email"`
	TelegramID int64  `json:"telegram_id"`
	Password   string `json:"password"`
}

func (h *UserHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := decodeJSON(w, r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	user, err := h.svc.Register(r.Context(), service.RegisterRequest{
		Name:       req.Name,
		Email:      req.Email,
		TelegramID: req.TelegramID,
		Password:   req.Password,
	})
	if err != nil {
		status, msg := mapServiceError(err)
		respondError(w, status, msg)
		return
	}

	respondJSON(w, http.StatusCreated, user)
}

// Login аутентифицирует пользователя и возвращает пару токенов. POST /auth/login
type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *UserHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(w, r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	tokens, err := h.svc.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		status, msg := mapServiceError(err)
		respondError(w, status, msg)
		return
	}

	respondJSON(w, http.StatusOK, tokens)
}

// Refresh обновляет пару токенов по refresh-токену. POST /auth/refresh
type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (h *UserHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := decodeJSON(w, r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	tokens, err := h.svc.RefreshToken(r.Context(), req.RefreshToken)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}

	respondJSON(w, http.StatusOK, tokens)
}

// GetAll возвращает список всех пользователей. GET /users (admin)
func (h *UserHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	users, err := h.svc.GetAll(r.Context())
	if err != nil {
		status, msg := mapServiceError(err)
		respondError(w, status, msg)
		return
	}

	respondJSON(w, http.StatusOK, users)
}

// GetByID возвращает пользователя по id. GET /users/{id} (admin)
func (h *UserHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	user, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		status, msg := mapServiceError(err)
		respondError(w, status, msg)
		return
	}

	respondJSON(w, http.StatusOK, user)
}

// Update частично обновляет пользователя. PATCH /users/{id}
// Пользователь может менять только свой профиль; роль — только администратор.
type updateRequest struct {
	Email    *string      `json:"email"`
	Name     *string      `json:"name"`
	Password *string      `json:"password"`
	Role     *domain.Role `json:"role"`
}

func (h *UserHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	var req updateRequest
	if err := decodeJSON(w, r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	params := service.UpdateUserParams{
		ID:       id,
		Email:    req.Email,
		Name:     req.Name,
		Password: req.Password,
		Role:     req.Role,
	}

	if err := h.svc.Update(r.Context(), params); err != nil {
		status, msg := mapServiceError(err)
		respondError(w, status, msg)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Delete удаляет пользователя по id. DELETE /users/{id} (admin)
func (h *UserHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	if err := h.svc.Delete(r.Context(), id); err != nil {
		status, msg := mapServiceError(err)
		respondError(w, status, msg)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
