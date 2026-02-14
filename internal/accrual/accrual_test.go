package accrual

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetOrderInfo_StatusOK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/orders/12345" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(OrderInfo{
			Order:   "12345",
			Status:  "PROCESSED",
			Accrual: 500,
		})
	}))
	defer server.Close()

	client := NewClient(server.URL)
	info, err := client.GetOrderInfo("12345")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil info")
	}
	if info.Status != "PROCESSED" {
		t.Errorf("expected PROCESSED, got %s", info.Status)
	}
	if info.Accrual != 500 {
		t.Errorf("expected accrual 500, got %f", info.Accrual)
	}
}

func TestGetOrderInfo_StatusNoContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	info, err := client.GetOrderInfo("12345")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info != nil {
		t.Error("expected nil info for 204")
	}
}

func TestGetOrderInfo_TooManyRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	info, err := client.GetOrderInfo("12345")
	if info != nil {
		t.Error("expected nil info for 429")
	}

	var tooMany *ErrTooManyRequests
	if !errors.As(err, &tooMany) {
		t.Fatalf("expected ErrTooManyRequests, got %v", err)
	}
	if tooMany.RetryAfter != 30*time.Second {
		t.Errorf("expected 30s retry, got %s", tooMany.RetryAfter)
	}
}

func TestGetOrderInfo_TooManyRequests_DefaultRetry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.GetOrderInfo("12345")

	var tooMany *ErrTooManyRequests
	if !errors.As(err, &tooMany) {
		t.Fatalf("expected ErrTooManyRequests, got %v", err)
	}
	if tooMany.RetryAfter != 60*time.Second {
		t.Errorf("expected 60s default retry, got %s", tooMany.RetryAfter)
	}
}

func TestGetOrderInfo_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.GetOrderInfo("12345")
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

func TestGetOrderInfo_ConnectionError(t *testing.T) {
	client := NewClient("http://localhost:1")
	_, err := client.GetOrderInfo("12345")
	if err == nil {
		t.Fatal("expected error for connection failure")
	}
}

func TestNewClient(t *testing.T) {
	client := NewClient("http://example.com")
	if client.baseURL != "http://example.com" {
		t.Errorf("expected baseURL http://example.com, got %s", client.baseURL)
	}
	if client.httpClient == nil {
		t.Fatal("httpClient should not be nil")
	}
}

func TestErrTooManyRequests_Error(t *testing.T) {
	err := &ErrTooManyRequests{RetryAfter: 30 * time.Second}
	msg := err.Error()
	if msg == "" {
		t.Fatal("error message should not be empty")
	}
}
