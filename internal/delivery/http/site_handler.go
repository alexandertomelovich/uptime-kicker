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

// SiteService описывает методы бизнес-логики, необходимые HTTP-слою.
// Интерфейс объявлен на стороне потребителя, чтобы упростить подмену в тестах.
type SiteService interface {
	Create(ctx context.Context, req service.CreateSiteRequest, userID uuid.UUID) (domain.Site, error)
	VerifySite(ctx context.Context, id, userID uuid.UUID, token string) error
	Delete(ctx context.Context, id, userID uuid.UUID) error
	GetAllSites(ctx context.Context, userID uuid.UUID) ([]domain.Site, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]domain.SiteResponse, error)
	GetByID(ctx context.Context, id uuid.UUID) (domain.Site, error)
	GetSiteStats(ctx context.Context, userID uuid.UUID) (domain.SiteStats, error)
	Update(ctx context.Context, update domain.SiteUpdate) (domain.Site, error)
}

// SiteHandler обслуживает HTTP-запросы, связанные с сайтами.
type SiteHandler struct {
	svc SiteService
}

func NewSiteHandler(svc SiteService) *SiteHandler {
	if svc == nil {
		panic("SiteHandler: svc is nil")
	}
	return &SiteHandler{svc: svc}
}

// Routes регистрирует маршруты handler-а на переданном роутере.
// authMiddleware — middleware аутентификации (например, JWTManager.AuthMiddleware).
func (h *SiteHandler) Routes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.Group(func(r chi.Router) {
		r.Use(authMiddleware)
		r.Post("/sites", h.Create)
		r.Get("/sites", h.GetAllSites)
		r.Get("/sites/mine", h.GetByUserID)
		r.Get("/sites/stats", h.GetStats)
		r.Get("/sites/{id}", h.GetByID)
		r.Patch("/sites/{id}", h.Update)
		r.Delete("/sites/{id}", h.Delete)
		r.Post("/sites/{id}/verify", h.Verify)
	})
}

// getUserID извлекает id аутентифицированного пользователя из контекста.
// При отсутствии возвращает false и уже пишет 401.
func getUserID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	claims, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, domain.ErrUnauthorized.Error())
		return uuid.Nil, false
	}
	return claims.UserID, true
}

// Create создаёт новый сайт для текущего пользователя. POST /sites
type createSiteRequest struct {
	Url                  string `json:"url"`
	Name                 string `json:"name"`
	CheckIntervalSeconds int    `json:"check_interval_seconds"`
}

func (h *SiteHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := getUserID(w, r)
	if !ok {
		return
	}

	var req createSiteRequest
	if err := decodeJSON(w, r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	site, err := h.svc.Create(r.Context(), service.CreateSiteRequest{
		Url:                  req.Url,
		Name:                 req.Name,
		CheckIntervalSeconds: req.CheckIntervalSeconds,
	}, userID)
	if err != nil {
		status, msg := mapServiceError(err)
		respondError(w, status, msg)
		return
	}

	respondJSON(w, http.StatusCreated, site)
}

// Verify подтверждает владение сайтом по токену. POST /sites/{id}/verify
type verifySiteRequest struct {
	Token string `json:"token"`
}

func (h *SiteHandler) Verify(w http.ResponseWriter, r *http.Request) {
	userID, ok := getUserID(w, r)
	if !ok {
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid site id")
		return
	}

	var req verifySiteRequest
	if err := decodeJSON(w, r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.svc.VerifySite(r.Context(), id, userID, req.Token); err != nil {
		status, msg := mapServiceError(err)
		respondError(w, status, msg)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetAllSites возвращает все сайты (admin). GET /sites
func (h *SiteHandler) GetAllSites(w http.ResponseWriter, r *http.Request) {
	userID, ok := getUserID(w, r)
	if !ok {
		return
	}

	sites, err := h.svc.GetAllSites(r.Context(), userID)
	if err != nil {
		status, msg := mapServiceError(err)
		respondError(w, status, msg)
		return
	}

	respondJSON(w, http.StatusOK, sites)
}

// GetByUserID возвращает сайты текущего пользователя. GET /sites/mine
func (h *SiteHandler) GetByUserID(w http.ResponseWriter, r *http.Request) {
	userID, ok := getUserID(w, r)
	if !ok {
		return
	}

	sites, err := h.svc.GetByUserID(r.Context(), userID)
	if err != nil {
		status, msg := mapServiceError(err)
		respondError(w, status, msg)
		return
	}

	respondJSON(w, http.StatusOK, sites)
}

// GetByID возвращает сайт по id. GET /sites/{id}
func (h *SiteHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid site id")
		return
	}

	site, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		status, msg := mapServiceError(err)
		respondError(w, status, msg)
		return
	}

	respondJSON(w, http.StatusOK, site)
}

// GetStats возвращает агрегированную статистику по сайтам пользователя. GET /sites/stats
func (h *SiteHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	userID, ok := getUserID(w, r)
	if !ok {
		return
	}

	stats, err := h.svc.GetSiteStats(r.Context(), userID)
	if err != nil {
		status, msg := mapServiceError(err)
		respondError(w, status, msg)
		return
	}

	respondJSON(w, http.StatusOK, stats)
}

// Update частично обновляет сайт. PATCH /sites/{id}
type updateSiteRequest struct {
	Url                  *string `json:"url"`
	Name                 *string `json:"name"`
	CheckIntervalSeconds *int    `json:"check_interval_seconds"`
	IsActive             *bool   `json:"is_active"`
}

func (h *SiteHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID, ok := getUserID(w, r)
	if !ok {
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid site id")
		return
	}

	var req updateSiteRequest
	if err := decodeJSON(w, r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	updated, err := h.svc.Update(r.Context(), domain.SiteUpdate{
		ID:                   id,
		UserID:               userID,
		Url:                  req.Url,
		Name:                 req.Name,
		CheckIntervalSeconds: req.CheckIntervalSeconds,
		IsActive:             req.IsActive,
	})
	if err != nil {
		status, msg := mapServiceError(err)
		respondError(w, status, msg)
		return
	}

	respondJSON(w, http.StatusOK, updated)
}

// Delete удаляет сайт по id. DELETE /sites/{id}
func (h *SiteHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := getUserID(w, r)
	if !ok {
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid site id")
		return
	}

	if err := h.svc.Delete(r.Context(), id, userID); err != nil {
		status, msg := mapServiceError(err)
		respondError(w, status, msg)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
