package auth

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const UserContextKey contextKey = "user"

// AuthMiddleware - middleware для проверки JWT токена
func (m *JWTManager) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Извлекаем токен из заголовка Authorization
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}

		// Проверяем формат: "Bearer <token>"
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			http.Error(w, "Invalid authorization header format", http.StatusUnauthorized)
			return
		}

		tokenString := parts[1]

		// Валидируем токен
		claims, err := m.ValidateAccessToken(tokenString)
		if err != nil {
			if err == ErrExpiredToken {
				http.Error(w, "Token expired", http.StatusUnauthorized)
				return
			}
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		// Сохраняем claims в контексте запроса
		ctx := context.WithValue(r.Context(), UserContextKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetUserFromContext - получает пользователя из контекста
func GetUserFromContext(ctx context.Context) (*Claims, bool) {
	claims, ok := ctx.Value(UserContextKey).(*Claims)
	return claims, ok
}

// RequireRole - middleware для проверки роли
func RequireRole(allowedRoles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := GetUserFromContext(r.Context())
			if !ok {
				http.Error(w, "User not authenticated", http.StatusUnauthorized)
				return
			}

			for _, role := range allowedRoles {
				if claims.Role == role {
					next.ServeHTTP(w, r)
					return
				}
			}

			http.Error(w, "Insufficient permissions", http.StatusForbidden)
		})
	}
}