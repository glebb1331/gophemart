package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/glebb1331/gophemart/internal/middleware"
	"github.com/glebb1331/gophemart/internal/storage"
	"golang.org/x/crypto/bcrypt"
)

// mockStorage реализует storage.Storage для тестирования.
type mockStorage struct {
	createUserFn         func(ctx context.Context, login, passwordHash string) (int, error)
	getUserByLoginFn     func(ctx context.Context, login string) (int, string, error)
	createOrderFn        func(ctx context.Context, userID int, orderNum string) error
	getUserOrdersFn      func(ctx context.Context, userID int) ([]storage.Order, error)
	getUserBalanceFn     func(ctx context.Context, userID int) (storage.Balance, error)
	withdrawFn           func(ctx context.Context, userID int, orderNum string, sum float64) error
	getUserWithdrawalsFn func(ctx context.Context, userID int) ([]storage.Withdrawal, error)
}

func (m *mockStorage) CreateUser(ctx context.Context, login, passwordHash string) (int, error) {
	return m.createUserFn(ctx, login, passwordHash)
}
func (m *mockStorage) GetUserByLogin(ctx context.Context, login string) (int, string, error) {
	return m.getUserByLoginFn(ctx, login)
}
func (m *mockStorage) CreateOrder(ctx context.Context, userID int, orderNum string) error {
	return m.createOrderFn(ctx, userID, orderNum)
}
func (m *mockStorage) GetUserOrders(ctx context.Context, userID int) ([]storage.Order, error) {
	return m.getUserOrdersFn(ctx, userID)
}
func (m *mockStorage) GetUserBalance(ctx context.Context, userID int) (storage.Balance, error) {
	return m.getUserBalanceFn(ctx, userID)
}
func (m *mockStorage) Withdraw(ctx context.Context, userID int, orderNum string, sum float64) error {
	return m.withdrawFn(ctx, userID, orderNum, sum)
}
func (m *mockStorage) GetUserWithdrawals(ctx context.Context, userID int) ([]storage.Withdrawal, error) {
	return m.getUserWithdrawalsFn(ctx, userID)
}
func (m *mockStorage) GetOrdersForProcessing(ctx context.Context, limit int) ([]storage.Order, error) {
	return nil, nil
}
func (m *mockStorage) ProcessAccrual(ctx context.Context, orderNum string, userID int, status string, accrual float64) error {
	return nil
}
func (m *mockStorage) Close() error { return nil }

func withUserID(r *http.Request, userID int) *http.Request {
	ctx := context.WithValue(r.Context(), middleware.UserIDKey, userID)
	return r.WithContext(ctx)
}

// --- Register tests ---

func TestRegister_Success(t *testing.T) {
	mock := &mockStorage{
		createUserFn: func(ctx context.Context, login, passwordHash string) (int, error) {
			return 1, nil
		},
	}
	h := NewHandlers(mock)

	body, _ := json.Marshal(map[string]string{"login": "user", "password": "pass"})
	req := httptest.NewRequest(http.MethodPost, "/api/user/register", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.Register(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if rr.Header().Get("Authorization") == "" {
		t.Error("expected Authorization header")
	}
}

func TestRegister_EmptyLogin(t *testing.T) {
	h := NewHandlers(&mockStorage{})

	body, _ := json.Marshal(map[string]string{"login": "", "password": "pass"})
	req := httptest.NewRequest(http.MethodPost, "/api/user/register", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.Register(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestRegister_Conflict(t *testing.T) {
	mock := &mockStorage{
		createUserFn: func(ctx context.Context, login, passwordHash string) (int, error) {
			return 0, storage.ErrConflict
		},
	}
	h := NewHandlers(mock)

	body, _ := json.Marshal(map[string]string{"login": "user", "password": "pass"})
	req := httptest.NewRequest(http.MethodPost, "/api/user/register", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.Register(rr, req)

	if rr.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", rr.Code)
	}
}

func TestRegister_InvalidJSON(t *testing.T) {
	h := NewHandlers(&mockStorage{})

	req := httptest.NewRequest(http.MethodPost, "/api/user/register", bytes.NewReader([]byte("invalid")))
	rr := httptest.NewRecorder()
	h.Register(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// --- Login tests ---

func TestLogin_Success(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("pass"), bcrypt.MinCost)
	mock := &mockStorage{
		getUserByLoginFn: func(ctx context.Context, login string) (int, string, error) {
			return 1, string(hash), nil
		},
	}
	h := NewHandlers(mock)

	body, _ := json.Marshal(map[string]string{"login": "user", "password": "pass"})
	req := httptest.NewRequest(http.MethodPost, "/api/user/login", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("correct"), bcrypt.MinCost)
	mock := &mockStorage{
		getUserByLoginFn: func(ctx context.Context, login string) (int, string, error) {
			return 1, string(hash), nil
		},
	}
	h := NewHandlers(mock)

	body, _ := json.Marshal(map[string]string{"login": "user", "password": "wrong"})
	req := httptest.NewRequest(http.MethodPost, "/api/user/login", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestLogin_UserNotFound(t *testing.T) {
	mock := &mockStorage{
		getUserByLoginFn: func(ctx context.Context, login string) (int, string, error) {
			return 0, "", storage.ErrConflict
		},
	}
	h := NewHandlers(mock)

	body, _ := json.Marshal(map[string]string{"login": "none", "password": "pass"})
	req := httptest.NewRequest(http.MethodPost, "/api/user/login", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestLogin_EmptyPassword(t *testing.T) {
	h := NewHandlers(&mockStorage{})

	body, _ := json.Marshal(map[string]string{"login": "user", "password": ""})
	req := httptest.NewRequest(http.MethodPost, "/api/user/login", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// --- UploadOrder tests ---

func TestUploadOrder_Success(t *testing.T) {
	mock := &mockStorage{
		createOrderFn: func(ctx context.Context, userID int, orderNum string) error {
			return nil
		},
	}
	h := NewHandlers(mock)

	req := httptest.NewRequest(http.MethodPost, "/api/user/orders", bytes.NewReader([]byte("12345678903")))
	req = withUserID(req, 1)
	rr := httptest.NewRecorder()
	h.UploadOrder(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", rr.Code)
	}
}

func TestUploadOrder_AlreadyByUser(t *testing.T) {
	mock := &mockStorage{
		createOrderFn: func(ctx context.Context, userID int, orderNum string) error {
			return storage.ErrOrderAlreadyUploadedByUser
		},
	}
	h := NewHandlers(mock)

	req := httptest.NewRequest(http.MethodPost, "/api/user/orders", bytes.NewReader([]byte("12345678903")))
	req = withUserID(req, 1)
	rr := httptest.NewRecorder()
	h.UploadOrder(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestUploadOrder_AlreadyByOther(t *testing.T) {
	mock := &mockStorage{
		createOrderFn: func(ctx context.Context, userID int, orderNum string) error {
			return storage.ErrOrderAlreadyUploadedByOther
		},
	}
	h := NewHandlers(mock)

	req := httptest.NewRequest(http.MethodPost, "/api/user/orders", bytes.NewReader([]byte("12345678903")))
	req = withUserID(req, 1)
	rr := httptest.NewRecorder()
	h.UploadOrder(rr, req)

	if rr.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", rr.Code)
	}
}

func TestUploadOrder_InvalidLuhn(t *testing.T) {
	h := NewHandlers(&mockStorage{})

	req := httptest.NewRequest(http.MethodPost, "/api/user/orders", bytes.NewReader([]byte("1234")))
	req = withUserID(req, 1)
	rr := httptest.NewRecorder()
	h.UploadOrder(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rr.Code)
	}
}

func TestUploadOrder_EmptyBody(t *testing.T) {
	h := NewHandlers(&mockStorage{})

	req := httptest.NewRequest(http.MethodPost, "/api/user/orders", bytes.NewReader([]byte("")))
	req = withUserID(req, 1)
	rr := httptest.NewRecorder()
	h.UploadOrder(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestUploadOrder_Unauthorized(t *testing.T) {
	h := NewHandlers(&mockStorage{})

	req := httptest.NewRequest(http.MethodPost, "/api/user/orders", bytes.NewReader([]byte("12345678903")))
	rr := httptest.NewRecorder()
	h.UploadOrder(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// --- GetOrders tests ---

func TestGetOrders_Success(t *testing.T) {
	mock := &mockStorage{
		getUserOrdersFn: func(ctx context.Context, userID int) ([]storage.Order, error) {
			return []storage.Order{
				{Number: "123", Status: storage.StatusProcessed, Accrual: 500, UploadedAt: time.Now()},
				{Number: "456", Status: storage.StatusNew, Accrual: 0, UploadedAt: time.Now()},
			}, nil
		},
	}
	h := NewHandlers(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/user/orders", nil)
	req = withUserID(req, 1)
	rr := httptest.NewRecorder()
	h.GetOrders(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if rr.Header().Get("Content-Type") != "application/json" {
		t.Error("expected application/json content type")
	}
}

func TestGetOrders_NoContent(t *testing.T) {
	mock := &mockStorage{
		getUserOrdersFn: func(ctx context.Context, userID int) ([]storage.Order, error) {
			return nil, nil
		},
	}
	h := NewHandlers(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/user/orders", nil)
	req = withUserID(req, 1)
	rr := httptest.NewRecorder()
	h.GetOrders(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rr.Code)
	}
}

// --- GetBalance tests ---

func TestGetBalance_Success(t *testing.T) {
	mock := &mockStorage{
		getUserBalanceFn: func(ctx context.Context, userID int) (storage.Balance, error) {
			return storage.Balance{Current: 500.5, Withdrawn: 42}, nil
		},
	}
	h := NewHandlers(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/user/balance", nil)
	req = withUserID(req, 1)
	rr := httptest.NewRecorder()
	h.GetBalance(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var balance storage.Balance
	json.NewDecoder(rr.Body).Decode(&balance)
	if balance.Current != 500.5 {
		t.Errorf("expected current 500.5, got %f", balance.Current)
	}
	if balance.Withdrawn != 42 {
		t.Errorf("expected withdrawn 42, got %f", balance.Withdrawn)
	}
}

// --- Withdraw tests ---

func TestWithdraw_Success(t *testing.T) {
	mock := &mockStorage{
		withdrawFn: func(ctx context.Context, userID int, orderNum string, sum float64) error {
			return nil
		},
	}
	h := NewHandlers(mock)

	body, _ := json.Marshal(map[string]interface{}{"order": "2377225624", "sum": 100})
	req := httptest.NewRequest(http.MethodPost, "/api/user/balance/withdraw", bytes.NewReader(body))
	req = withUserID(req, 1)
	rr := httptest.NewRecorder()
	h.Withdraw(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestWithdraw_InsufficientFunds(t *testing.T) {
	mock := &mockStorage{
		withdrawFn: func(ctx context.Context, userID int, orderNum string, sum float64) error {
			return storage.ErrInsufficientFunds
		},
	}
	h := NewHandlers(mock)

	body, _ := json.Marshal(map[string]interface{}{"order": "2377225624", "sum": 100})
	req := httptest.NewRequest(http.MethodPost, "/api/user/balance/withdraw", bytes.NewReader(body))
	req = withUserID(req, 1)
	rr := httptest.NewRecorder()
	h.Withdraw(rr, req)

	if rr.Code != http.StatusPaymentRequired {
		t.Errorf("expected 402, got %d", rr.Code)
	}
}

func TestWithdraw_InvalidOrder(t *testing.T) {
	h := NewHandlers(&mockStorage{})

	body, _ := json.Marshal(map[string]interface{}{"order": "1111", "sum": 100})
	req := httptest.NewRequest(http.MethodPost, "/api/user/balance/withdraw", bytes.NewReader(body))
	req = withUserID(req, 1)
	rr := httptest.NewRecorder()
	h.Withdraw(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rr.Code)
	}
}

// --- GetWithdrawals tests ---

func TestGetWithdrawals_Success(t *testing.T) {
	mock := &mockStorage{
		getUserWithdrawalsFn: func(ctx context.Context, userID int) ([]storage.Withdrawal, error) {
			return []storage.Withdrawal{
				{Order: "123", Sum: 100, ProcessedAt: time.Now()},
			}, nil
		},
	}
	h := NewHandlers(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/user/withdrawals", nil)
	req = withUserID(req, 1)
	rr := httptest.NewRecorder()
	h.GetWithdrawals(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestGetWithdrawals_NoContent(t *testing.T) {
	mock := &mockStorage{
		getUserWithdrawalsFn: func(ctx context.Context, userID int) ([]storage.Withdrawal, error) {
			return nil, nil
		},
	}
	h := NewHandlers(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/user/withdrawals", nil)
	req = withUserID(req, 1)
	rr := httptest.NewRecorder()
	h.GetWithdrawals(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rr.Code)
	}
}

// --- isValidLuhn tests ---

func TestIsValidLuhn(t *testing.T) {
	tests := []struct {
		number string
		valid  bool
	}{
		{"12345678903", true},
		{"2377225624", true},
		{"9278923470", true},
		{"0", true},
		{"1234", false},
		{"", false},
		{"abc", false},
		{"1234567890", false},
	}

	for _, tt := range tests {
		t.Run(tt.number, func(t *testing.T) {
			if got := isValidLuhn(tt.number); got != tt.valid {
				t.Errorf("isValidLuhn(%q) = %v, want %v", tt.number, got, tt.valid)
			}
		})
	}
}
