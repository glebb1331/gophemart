package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/glebb1331/gophemart/internal/accrual"
	"github.com/glebb1331/gophemart/internal/auth"
	"github.com/glebb1331/gophemart/internal/config"
	"github.com/glebb1331/gophemart/internal/handlers"
	"github.com/glebb1331/gophemart/internal/middleware"
	"github.com/glebb1331/gophemart/internal/storage"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.NewConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if cfg.DatabaseURI == "" {
		return fmt.Errorf("database URI is required")
	}

	auth.SetSecret(cfg.JWTSecretKey)

	store, err := storage.NewDatabaseStorage(cfg.DatabaseURI)
	if err != nil {
		return fmt.Errorf("failed to init storage: %w", err)
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
		pollInterval := time.Duration(cfg.PollInterval) * time.Second
		worker := accrual.NewWorker(accrualClient, store, pollInterval)
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
		return fmt.Errorf("shutdown error: %w", err)
	}
	log.Println("server stopped")
	return nil
}
