package auth

import (
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

func TestMain(m *testing.M) {
	SetSecret("testsecretkey")
	os.Exit(m.Run())
}

func TestGenerateToken(t *testing.T) {
	token, err := GenerateToken(42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token == "" {
		t.Fatal("token should not be empty")
	}
}

func TestParseToken_Valid(t *testing.T) {
	token, err := GenerateToken(123)
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}

	claims, err := ParseToken(token)
	if err != nil {
		t.Fatalf("unexpected error parsing token: %v", err)
	}
	if claims.UserID != 123 {
		t.Errorf("expected userID 123, got %d", claims.UserID)
	}
}

func TestParseToken_InvalidString(t *testing.T) {
	_, err := ParseToken("invalid-token")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

func TestParseToken_ExpiredToken(t *testing.T) {
	claims := Claims{
		UserID: 1,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString(jwtSecret)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = ParseToken(tokenStr)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestParseToken_WrongSigningMethod(t *testing.T) {
	claims := Claims{
		UserID: 1,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	tokenStr, _ := token.SignedString(jwt.UnsafeAllowNoneSignatureType)

	_, err := ParseToken(tokenStr)
	if err == nil {
		t.Fatal("expected error for wrong signing method")
	}
}

func TestGenerateToken_DifferentUsers(t *testing.T) {
	token1, _ := GenerateToken(1)
	token2, _ := GenerateToken(2)
	if token1 == token2 {
		t.Error("tokens for different users should be different")
	}
}
