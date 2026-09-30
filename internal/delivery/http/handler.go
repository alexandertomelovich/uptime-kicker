package httphandler

import (
	"encoding/json"
	"errors"
	"health_checker/internal/domain"
	"log"
	"net/http"
)

// APIError — единый формат ошибки HTTP-ответа.
type APIError struct {
	Error string `json:"error"`
}

// respondJSON сериализует payload в JSON и пишет ответ с указанным статусом.
func respondJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("http.respondJSON: failed to encode response: %v", err)
	}
}

// respondError пишет JSON-ошибку.
func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, APIError{Error: message})
}

// decodeJSON читает тело запроса в dst, ограничивая размер 1 МиБ.
// Лишние (неизвестные) поля считаются ошибкой.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

// mapServiceError превращает domain-ошибку в HTTP-статус и текст ответа.
func mapServiceError(err error) (int, string) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound, domain.ErrNotFound.Error()
	case errors.Is(err, domain.ErrUnauthorized):
		return http.StatusUnauthorized, domain.ErrUnauthorized.Error()
	case errors.Is(err, domain.ErrInvalidCredentials):
		return http.StatusUnauthorized, domain.ErrInvalidCredentials.Error()
	case errors.Is(err, domain.ErrAccessDenied):
		return http.StatusForbidden, domain.ErrAccessDenied.Error()
	case errors.Is(err, domain.ErrRoleChangeForbidden):
		return http.StatusForbidden, domain.ErrRoleChangeForbidden.Error()
	case errors.Is(err, domain.ErrSiteNotBelongUser):
		return http.StatusForbidden, err.Error()
	case errors.Is(err, domain.ErrSiteAlreadyVerified):
		return http.StatusConflict, err.Error()
	case errors.Is(err, domain.ErrEmailAlreadyExists),
		errors.Is(err, domain.ErrTelegramAlreadyExists),
		errors.Is(err, domain.ErrPasswordPolicy),
		errors.Is(err, domain.ErrInvalidRole),
		errors.Is(err, domain.ErrSiteNotVerified),
		errors.Is(err, domain.ErrSiteNotActive),
		errors.Is(err, domain.ErrInvalidToken):
		return http.StatusBadRequest, err.Error()
	default:
		log.Printf("http.mapServiceError: unexpected error: %v", err)
		return http.StatusInternalServerError, "internal server error"
	}
}
