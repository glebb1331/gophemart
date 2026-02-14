// Package auth реализует генерацию и валидацию JWT-токенов
// для аутентификации пользователей в системе Гофермарт.
package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

// Claims описывает пользовательские данные, хранящиеся в JWT-токене.
type Claims struct {
	// UserID — идентификатор пользователя.
	UserID int `json:"user_id"`
	jwt.RegisteredClaims
}

const secretKey = "supersecretkey"

// GenerateToken создаёт JWT-токен для пользователя с указанным идентификатором.
// Токен действителен 24 часа с момента создания.
func GenerateToken(userID int) (string, error) {
	claims := Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secretKey))
}

// ParseToken проверяет и разбирает JWT-токен, возвращая данные пользователя.
// Возвращает ошибку, если токен невалиден, истёк или подписан неизвестным методом.
func ParseToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secretKey), nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}
	return nil, jwt.ErrSignatureInvalid
}
