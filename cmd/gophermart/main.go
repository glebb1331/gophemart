package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/glebb1331/gophemart/internal/accrual"
	"github.com/glebb1331/gophemart/internal/config"
	"github.com/glebb1331/gophemart/internal/handlers"
	"github.com/glebb1331/gophemart/internal/middleware"
	"github.com/glebb1331/gophemart/internal/storage"
)

func main() {
	cfg, err := config.NewConfig()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	if cfg.DatabaseURI == "" {
		log.Fatal("database URI is required")
	}

	store, err := storage.NewDatabaseStorage(cfg.DatabaseURI)
	if err != nil {
		log.Fatalf("failed to init storage: %v", err)
	}
	defer store.Close()

	h := handlers.NewHandlers(store)

	r := chi.NewRouter()

	r.Post("/api/user/register", h.Register)
	r.Post("/api/user/login", h.Login)

	r.Group(func(r chi.Router) {
		r.Use(middleware.AuthMiddleware)

		r.Post("/api/user/orders", h.UploadOrder)
		r.Get("/api/user/orders", h.GetOrders)
		r.Get("/api/user/balance", h.GetBalance)
		r.Post("/api/user/balance/withdraw", h.Withdraw)
		r.Get("/api/user/withdrawals", h.GetWithdrawals)
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if cfg.AccrualSystemAddress != "" {
		accrualClient := accrual.NewClient(cfg.AccrualSystemAddress)
		worker := accrual.NewWorker(accrualClient, store)
		go worker.Run(ctx)
	}

	srv := &http.Server{
		Addr:    cfg.RunAddress,
		Handler: r,
	}

	go func() {
		log.Printf("starting server on %s", cfg.RunAddress)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("shutdown error: %v", err)
	}
	log.Println("server stopped")
}
