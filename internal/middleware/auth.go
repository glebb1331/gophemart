// Package middleware содержит HTTP-мидлвари для сервиса Гофермарт.
// Включает middleware для проверки аутентификации пользователей.
package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/glebb1331/gophemart/internal/auth"
)

type contextKey string

// UserIDKey — ключ для хранения идентификатора пользователя в контексте запроса.
const UserIDKey contextKey = "userID"

// AuthMiddleware проверяет аутентификацию пользователя по JWT-токену.
// Токен извлекается из заголовка Authorization (Bearer) или из cookie "token".
// При успешной проверке userID помещается в контекст запроса.
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var tokenStr string

		if authHeader := r.Header.Get("Authorization"); authHeader != "" {
			tokenStr = strings.TrimPrefix(authHeader, "Bearer ")
		}

		if tokenStr == "" {
			cookie, err := r.Cookie("token")
			if err != nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			tokenStr = cookie.Value
		}

		claims, err := auth.ParseToken(tokenStr)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), UserIDKey, claims.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetUserID извлекает идентификатор пользователя из контекста HTTP-запроса.
// Возвращает userID и true, если значение найдено, иначе 0 и false.
func GetUserID(r *http.Request) (int, bool) {
	val := r.Context().Value(UserIDKey)
	if val == nil {
		return 0, false
	}
	userID, ok := val.(int)
	return userID, ok
}
