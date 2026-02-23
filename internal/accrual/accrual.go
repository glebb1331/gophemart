// Package accrual реализует взаимодействие с внешней системой расчёта начислений
// баллов лояльности и фоновую обработку заказов.
package accrual

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// Client — HTTP-клиент для взаимодействия с системой расчёта начислений.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// OrderInfo содержит информацию о начислении баллов за заказ от внешней системы.
type OrderInfo struct {
	// Order — номер заказа.
	Order string `json:"order"`
	// Status — статус расчёта начисления.
	Status string `json:"status"`
	// Accrual — рассчитанные баллы к начислению.
	Accrual float64 `json:"accrual,omitempty"`
}

// NewClient создаёт нового клиента для системы начислений с указанным базовым URL.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GetOrderInfo запрашивает информацию о начислении для указанного заказа.
// Возвращает nil, nil если заказ не зарегистрирован в системе (204).
// Возвращает ErrTooManyRequests при превышении лимита запросов (429).
func (c *Client) GetOrderInfo(orderNum string) (*OrderInfo, error) {
	url := fmt.Sprintf("%s/api/orders/%s", c.baseURL, orderNum)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var info OrderInfo
		if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
			return nil, err
		}
		return &info, nil
	case http.StatusNoContent:
		return nil, nil
	case http.StatusTooManyRequests:
		retryAfter := 60
		if val := resp.Header.Get("Retry-After"); val != "" {
			if parsed, err := strconv.Atoi(val); err == nil {
				retryAfter = parsed
			}
		}
		return nil, &ErrTooManyRequests{RetryAfter: time.Duration(retryAfter) * time.Second}
	default:
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}
}

// ErrTooManyRequests возвращается при превышении лимита запросов к системе начислений.
type ErrTooManyRequests struct {
	// RetryAfter — рекомендуемое время ожидания перед повторным запросом.
	RetryAfter time.Duration
}

// Error возвращает текстовое описание ошибки.
func (e *ErrTooManyRequests) Error() string {
	return fmt.Sprintf("too many requests, retry after %s", e.RetryAfter)
}
