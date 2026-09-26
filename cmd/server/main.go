package main

import (
	"context"
	"errors"
	"health_checker/config"
	"health_checker/internal/repository"
	"health_checker/internal/repository/postgres"
	checkersvc "health_checker/internal/service/checker"
	cronsvc "health_checker/internal/service/cron"
	"health_checker/internal/telegram"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("server exited with error: %v", err)
	}
}

func run() error {
	cfg := config.LoadConfig()

	// Корневой контекст, отменяется по SIGINT/SIGTERM — база для graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// --- База данных ---
	pool, err := pgxpool.New(ctx, cfg.DB.URL)
	if err != nil {
		return err
	}
	defer pool.Close()

	pingCtx, cancelPing := context.WithTimeout(ctx, 5*time.Second)
	defer cancelPing()
	if err := pool.Ping(pingCtx); err != nil {
		return err
	}
	log.Println("database connection established")

	queries := postgres.New(pool)

	// --- Репозитории ---
	// userRepo понадобится при подключении HTTP-handlers (auth/user).
	siteRepo := repository.NewSiteRepository(queries)
	checkRepo := repository.NewCheckRepository(queries)

	// --- Telegram ---
	bot, err := tgbotapi.NewBotAPI(cfg.Telegram.Token)
	if err != nil {
		return err
	}
	sender := telegram.NewSender(bot)
	log.Printf("telegram bot authorized as @%s", bot.Self.UserName)

	// --- Сервисы ---
	// userService и siteService (auth/site) будут подключены к HTTP-handlers
	// на следующем шаге; для каркаса достаточно checker и cron.
	checker := checkersvc.NewCheckerService(
		siteRepo,
		checkRepo,
		sender,
		cfg.Checker.NumWorkers,
		cfg.Checker.Limit,
		cfg.Checker.QueueSize,
	)
	checker.Start()

	dailyStats := cronsvc.NewDailyStatsService(checkRepo)
	if err := dailyStats.StartCron(); err != nil {
		return err
	}

	// --- HTTP ---
	router := newRouter()
	srv := &http.Server{
		Addr:    ":" + cfg.HTTP.Port,
		Handler: router,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Printf("http server listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// --- Ожидание остановки ---
	select {
	case <-ctx.Done():
		log.Println("shutdown signal received")
	case err := <-serverErr:
		log.Printf("http server error: %v", err)
		return err
	}

	// --- Graceful shutdown ---
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelShutdown()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("http server shutdown error: %v", err)
	}

	checker.Stop()
	dailyStats.StopCron(shutdownCtx)

	log.Println("shutdown complete")
	return nil
}

// newRouter собирает роутер chi с базовыми middleware.
// Бизнес-роуты будут добавлены на следующем шаге.
func newRouter() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("ok")); err != nil {
			log.Printf("healthz: failed to write response: %v", err)
		}
	})

	return r
}
