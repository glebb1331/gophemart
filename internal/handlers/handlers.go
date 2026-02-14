// Package handlers реализует HTTP-обработчики для API сервиса Гофермарт.
// Включает регистрацию, аутентификацию, управление заказами и балансом.
package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/glebb1331/gophemart/internal/auth"
	"github.com/glebb1331/gophemart/internal/middleware"
	"github.com/glebb1331/gophemart/internal/storage"
	"golang.org/x/crypto/bcrypt"
)

// Handlers содержит зависимости для HTTP-обработчиков.
type Handlers struct {
	storage storage.Storage
}

// NewHandlers создаёт экземпляр Handlers с указанным хранилищем.
func NewHandlers(s storage.Storage) *Handlers {
	return &Handlers{
		storage: s,
	}
}

func setAuthCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    token,
		HttpOnly: true,
		Path:     "/",
	})
	w.Header().Set("Authorization", "Bearer "+token)
}

// Register обрабатывает регистрацию нового пользователя.
// При успешной регистрации выполняет автоматическую аутентификацию.
func (h *Handlers) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	if req.Login == "" || req.Password == "" {
		http.Error(w, "Login and password are required", http.StatusBadRequest)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	userID, err := h.storage.CreateUser(r.Context(), req.Login, string(hash))
	if err != nil {
		if errors.Is(err, storage.ErrConflict) {
			http.Error(w, "Login already taken", http.StatusConflict)
			return
		}
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	token, err := auth.GenerateToken(userID)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	setAuthCookie(w, token)
	w.WriteHeader(http.StatusOK)
}

// Login обрабатывает аутентификацию пользователя по логину и паролю.
func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	if req.Login == "" || req.Password == "" {
		http.Error(w, "Login and password are required", http.StatusBadRequest)
		return
	}

	userID, hash, err := h.storage.GetUserByLogin(r.Context(), req.Login)
	if err != nil {
		http.Error(w, "Invalid login/password", http.StatusUnauthorized)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)); err != nil {
		http.Error(w, "Invalid login/password", http.StatusUnauthorized)
		return
	}

	token, err := auth.GenerateToken(userID)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	setAuthCookie(w, token)
	w.WriteHeader(http.StatusOK)
}

// UploadOrder обрабатывает загрузку номера заказа пользователем.
// Проверяет номер по алгоритму Луна.
func (h *Handlers) UploadOrder(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	orderNum := strings.TrimSpace(string(body))

	if orderNum == "" {
		http.Error(w, "Empty order number", http.StatusBadRequest)
		return
	}

	if !isValidLuhn(orderNum) {
		http.Error(w, "Invalid order number format", http.StatusUnprocessableEntity)
		return
	}

	err = h.storage.CreateOrder(r.Context(), userID, orderNum)
	if err != nil {
		switch {
		case errors.Is(err, storage.ErrOrderAlreadyUploadedByUser):
			w.WriteHeader(http.StatusOK)
		case errors.Is(err, storage.ErrOrderAlreadyUploadedByOther):
			http.Error(w, "Order number already uploaded by another user", http.StatusConflict)
		default:
			http.Error(w, "Internal error", http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

// GetOrders возвращает список заказов пользователя в формате JSON.
func (h *Handlers) GetOrders(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	orders, err := h.storage.GetUserOrders(r.Context(), userID)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	if len(orders) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	type orderResponse struct {
		Number     string   `json:"number"`
		Status     string   `json:"status"`
		Accrual    *float64 `json:"accrual,omitempty"`
		UploadedAt string   `json:"uploaded_at"`
	}
	resp := make([]orderResponse, 0, len(orders))
	for _, o := range orders {
		or := orderResponse{
			Number:     o.Number,
			Status:     o.Status,
			UploadedAt: o.UploadedAt.Format(time.RFC3339),
		}
		if o.Status == storage.StatusProcessed && o.Accrual > 0 {
			accrual := o.Accrual
			or.Accrual = &accrual
		}
		resp = append(resp, or)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// GetBalance возвращает текущий баланс баллов лояльности пользователя.
func (h *Handlers) GetBalance(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	balance, err := h.storage.GetUserBalance(r.Context(), userID)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(balance)
}

// Withdraw обрабатывает запрос на списание баллов со счёта пользователя.
func (h *Handlers) Withdraw(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Order string  `json:"order"`
		Sum   float64 `json:"sum"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	if !isValidLuhn(req.Order) {
		http.Error(w, "Invalid order number", http.StatusUnprocessableEntity)
		return
	}

	err := h.storage.Withdraw(r.Context(), userID, req.Order, req.Sum)
	if err != nil {
		switch {
		case errors.Is(err, storage.ErrInsufficientFunds):
			http.Error(w, "Insufficient funds", http.StatusPaymentRequired)
		default:
			http.Error(w, "Internal error", http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
}

// GetWithdrawals возвращает историю списаний баллов пользователя.
func (h *Handlers) GetWithdrawals(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	withdrawals, err := h.storage.GetUserWithdrawals(r.Context(), userID)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	if len(withdrawals) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	type withdrawalResponse struct {
		Order       string  `json:"order"`
		Sum         float64 `json:"sum"`
		ProcessedAt string  `json:"processed_at"`
	}
	resp := make([]withdrawalResponse, 0, len(withdrawals))
	for _, wd := range withdrawals {
		resp = append(resp, withdrawalResponse{
			Order:       wd.Order,
			Sum:         wd.Sum,
			ProcessedAt: wd.ProcessedAt.Format(time.RFC3339),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func isValidLuhn(number string) bool {
	number = strings.ReplaceAll(number, " ", "")
	if number == "" {
		return false
	}
	for _, c := range number {
		if c < '0' || c > '9' {
			return false
		}
	}

	sum := 0
	alternate := false
	for i := len(number) - 1; i >= 0; i-- {
		n, _ := strconv.Atoi(string(number[i]))
		if alternate {
			n *= 2
			if n > 9 {
				n -= 9
			}
		}
		sum += n
		alternate = !alternate
	}
	return sum%10 == 0
}
