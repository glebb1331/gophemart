package accrual

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/glebb1331/gophemart/internal/storage"
)

// Worker выполняет фоновый опрос системы начислений для обновления статусов заказов.
type Worker struct {
	client       *Client
	storage      storage.Storage
	pollInterval time.Duration
}

// NewWorker создаёт воркер для фоновой обработки начислений.
func NewWorker(client *Client, s storage.Storage, pollInterval time.Duration) *Worker {
	return &Worker{
		client:       client,
		storage:      s,
		pollInterval: pollInterval,
	}
}

// Run запускает цикл опроса системы начислений.
// Блокирует выполнение до отмены контекста.
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.processOrders(ctx)
		}
	}
}

func (w *Worker) processOrders(ctx context.Context) {
	orders, err := w.storage.GetOrdersForProcessing(ctx, 10)
	if err != nil {
		log.Printf("accrual worker: failed to get orders: %v", err)
		return
	}

	for _, order := range orders {
		select {
		case <-ctx.Done():
			return
		default:
		}

		info, err := w.client.GetOrderInfo(order.Number)
		if err != nil {
			var tooMany *ErrTooManyRequests
			if errors.As(err, &tooMany) {
				log.Printf("accrual worker: rate limited, waiting %s", tooMany.RetryAfter)
				select {
				case <-ctx.Done():
					return
				case <-time.After(tooMany.RetryAfter):
				}
				return
			}
			log.Printf("accrual worker: error getting order %s info: %v", order.Number, err)
			continue
		}

		if info == nil {
			continue
		}

		var newStatus string
		switch info.Status {
		case "REGISTERED":
			newStatus = storage.StatusNew
		case "PROCESSING":
			newStatus = storage.StatusProcessing
		case "INVALID":
			newStatus = storage.StatusInvalid
		case "PROCESSED":
			newStatus = storage.StatusProcessed
		default:
			log.Printf("accrual worker: unknown status %s for order %s", info.Status, order.Number)
			continue
		}

		if err := w.storage.ProcessAccrual(ctx, order.Number, order.UserID, newStatus, info.Accrual); err != nil {
			log.Printf("accrual worker: failed to process order %s: %v", order.Number, err)
		}
	}
}
