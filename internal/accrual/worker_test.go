package accrual

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/glebb1331/gophemart/internal/storage"
)

type mockStorage struct {
	mu                sync.Mutex
	orders            []storage.Order
	processedOrders   []processedCall
	getOrdersErr      error
	processAccrualErr error
}

type processedCall struct {
	orderNum string
	userID   int
	status   string
	accrual  float64
}

func (m *mockStorage) GetOrdersForProcessing(ctx context.Context, limit int) ([]storage.Order, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.orders, m.getOrdersErr
}

func (m *mockStorage) ProcessAccrual(ctx context.Context, orderNum string, userID int, status string, accrual float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.processedOrders = append(m.processedOrders, processedCall{orderNum, userID, status, accrual})
	return m.processAccrualErr
}

func (m *mockStorage) CreateUser(ctx context.Context, login, passwordHash string) (int, error) {
	return 0, nil
}
func (m *mockStorage) GetUserByLogin(ctx context.Context, login string) (int, string, error) {
	return 0, "", nil
}
func (m *mockStorage) CreateOrder(ctx context.Context, userID int, orderNum string) error {
	return nil
}
func (m *mockStorage) GetUserOrders(ctx context.Context, userID int) ([]storage.Order, error) {
	return nil, nil
}
func (m *mockStorage) GetUserBalance(ctx context.Context, userID int) (storage.Balance, error) {
	return storage.Balance{}, nil
}
func (m *mockStorage) Withdraw(ctx context.Context, userID int, orderNum string, sum float64) error {
	return nil
}
func (m *mockStorage) GetUserWithdrawals(ctx context.Context, userID int) ([]storage.Withdrawal, error) {
	return nil, nil
}
func (m *mockStorage) Close() error { return nil }

func TestWorker_ProcessOrders_Processed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(OrderInfo{
			Order:   "12345",
			Status:  "PROCESSED",
			Accrual: 100,
		})
	}))
	defer server.Close()

	mock := &mockStorage{
		orders: []storage.Order{
			{ID: 1, UserID: 42, Number: "12345", Status: storage.StatusNew, UploadedAt: time.Now()},
		},
	}

	client := NewClient(server.URL)
	w := NewWorker(client, mock)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w.processOrders(ctx)

	mock.mu.Lock()
	defer mock.mu.Unlock()

	if len(mock.processedOrders) != 1 {
		t.Fatalf("expected 1 processed order, got %d", len(mock.processedOrders))
	}
	call := mock.processedOrders[0]
	if call.status != storage.StatusProcessed {
		t.Errorf("expected PROCESSED, got %s", call.status)
	}
	if call.accrual != 100 {
		t.Errorf("expected accrual 100, got %f", call.accrual)
	}
	if call.userID != 42 {
		t.Errorf("expected userID 42, got %d", call.userID)
	}
}

func TestWorker_ProcessOrders_Invalid(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(OrderInfo{
			Order:  "12345",
			Status: "INVALID",
		})
	}))
	defer server.Close()

	mock := &mockStorage{
		orders: []storage.Order{
			{ID: 1, UserID: 1, Number: "12345", Status: storage.StatusNew, UploadedAt: time.Now()},
		},
	}

	client := NewClient(server.URL)
	w := NewWorker(client, mock)
	w.processOrders(context.Background())

	mock.mu.Lock()
	defer mock.mu.Unlock()

	if len(mock.processedOrders) != 1 {
		t.Fatalf("expected 1 processed order, got %d", len(mock.processedOrders))
	}
	if mock.processedOrders[0].status != storage.StatusInvalid {
		t.Errorf("expected INVALID, got %s", mock.processedOrders[0].status)
	}
}

func TestWorker_ProcessOrders_Registered(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(OrderInfo{
			Order:  "12345",
			Status: "REGISTERED",
		})
	}))
	defer server.Close()

	mock := &mockStorage{
		orders: []storage.Order{
			{ID: 1, UserID: 1, Number: "12345", Status: storage.StatusNew, UploadedAt: time.Now()},
		},
	}

	client := NewClient(server.URL)
	w := NewWorker(client, mock)
	w.processOrders(context.Background())

	mock.mu.Lock()
	defer mock.mu.Unlock()

	if len(mock.processedOrders) != 1 {
		t.Fatalf("expected 1 processed order, got %d", len(mock.processedOrders))
	}
	if mock.processedOrders[0].status != storage.StatusNew {
		t.Errorf("expected NEW, got %s", mock.processedOrders[0].status)
	}
}

func TestWorker_ProcessOrders_NoContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	mock := &mockStorage{
		orders: []storage.Order{
			{ID: 1, UserID: 1, Number: "12345", Status: storage.StatusNew, UploadedAt: time.Now()},
		},
	}

	client := NewClient(server.URL)
	w := NewWorker(client, mock)
	w.processOrders(context.Background())

	mock.mu.Lock()
	defer mock.mu.Unlock()

	if len(mock.processedOrders) != 0 {
		t.Errorf("expected 0 processed orders for 204, got %d", len(mock.processedOrders))
	}
}

func TestWorker_ProcessOrders_TooManyRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	mock := &mockStorage{
		orders: []storage.Order{
			{ID: 1, UserID: 1, Number: "111", Status: storage.StatusNew, UploadedAt: time.Now()},
			{ID: 2, UserID: 1, Number: "222", Status: storage.StatusNew, UploadedAt: time.Now()},
		},
	}

	client := NewClient(server.URL)
	w := NewWorker(client, mock)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	w.processOrders(ctx)

	mock.mu.Lock()
	defer mock.mu.Unlock()

	if len(mock.processedOrders) != 0 {
		t.Errorf("expected 0 processed orders on 429, got %d", len(mock.processedOrders))
	}
}

func TestWorker_ProcessOrders_EmptyList(t *testing.T) {
	mock := &mockStorage{orders: nil}
	client := NewClient("http://localhost:1")
	w := NewWorker(client, mock)
	w.processOrders(context.Background())
}

func TestWorker_Run_CancelContext(t *testing.T) {
	mock := &mockStorage{}
	client := NewClient("http://localhost:1")
	w := NewWorker(client, mock)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not stop after context cancel")
	}
}

func TestWorker_ProcessOrders_Processing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(OrderInfo{
			Order:  "12345",
			Status: "PROCESSING",
		})
	}))
	defer server.Close()

	mock := &mockStorage{
		orders: []storage.Order{
			{ID: 1, UserID: 1, Number: "12345", Status: storage.StatusNew, UploadedAt: time.Now()},
		},
	}

	client := NewClient(server.URL)
	w := NewWorker(client, mock)
	w.processOrders(context.Background())

	mock.mu.Lock()
	defer mock.mu.Unlock()

	if len(mock.processedOrders) != 1 {
		t.Fatalf("expected 1 processed order, got %d", len(mock.processedOrders))
	}
	if mock.processedOrders[0].status != storage.StatusProcessing {
		t.Errorf("expected PROCESSING, got %s", mock.processedOrders[0].status)
	}
}
