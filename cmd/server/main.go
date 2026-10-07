package main

import (
	"context"
	"errors"
	"health_checker/config"
	"health_checker/internal/auth"
	httphandler "health_checker/internal/delivery/http"
	"health_checker/internal/repository"
	"health_checker/internal/repository/postgres"
	appservice "health_checker/internal/service"
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

	siteRepo := repository.NewSiteRepository(queries)
	checkRepo := repository.NewCheckRepository(queries)
	userRepo := repository.NewUserRepository(queries)
	refreshTokenRepo := repository.NewRefreshTokenRepository(queries)

	bot, err := tgbotapi.NewBotAPI(cfg.Telegram.Token)
	if err != nil {
		return err
	}
	sender := telegram.NewSender(bot)
	log.Printf("telegram bot authorized as @%s", bot.Self.UserName)

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

	jwtManager := auth.NewJWTManager(
		cfg.JWT.AccessSecret,
		cfg.JWT.RefreshSecret,
		cfg.JWT.AccessTTL,
		cfg.JWT.RefreshTTL,
		auth.WithTokenStore(refreshTokenRepo),
		auth.WithUserProvider(userRepo),
	)
	userService := appservice.NewUserService(userRepo, sender, jwtManager, refreshTokenRepo)
	userHandler := httphandler.NewUserHandler(userService)

	siteService := appservice.NewSiteService(siteRepo, userRepo)
	siteHandler := httphandler.NewSiteHandler(siteService)

	// --- Telegram-бот (long-polling) ---
	// Пользователь резолвится по telegram_id напрямую через репозиторий
	// (метод без admin-claims), а операции над сайтами идут через siteService.
	telegramHandler := telegram.NewHandler(bot, userRepo, siteService)
	telegramBot := telegram.NewBot(bot, telegramHandler)

	router := newRouter(userHandler, siteHandler, jwtManager.AuthMiddleware)
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

	// --- Запуск приёма сообщений Telegram ---
	// Бот работает в отдельной горутине и останавливается по отмене botCtx.
	botCtx, cancelBot := context.WithCancel(context.Background())
	defer cancelBot()

	botErr := make(chan error, 1)
	go func() {
		if err := telegramBot.Run(botCtx); err != nil && !errors.Is(err, context.Canceled) {
			botErr <- err
		}
	}()
	log.Println("telegram bot started (long-polling)")

	select {
	case <-ctx.Done():
		log.Println("shutdown signal received")
	case err := <-serverErr:
		log.Printf("http server error: %v", err)
		return err
	case err := <-botErr:
		log.Printf("telegram bot error: %v", err)
		return err
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelShutdown()

	// Останавливаем приём обновлений Telegram до закрытия пула БД.
	cancelBot()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("http server shutdown error: %v", err)
	}

	checker.Stop()
	dailyStats.StopCron(shutdownCtx)

	log.Println("shutdown complete")
	return nil
}

func newRouter(userHandler *httphandler.UserHandler, siteHandler *httphandler.SiteHandler, authMiddleware func(http.Handler) http.Handler) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("ok")); err != nil {
			log.Printf("healthz: failed to write response: %v", err)
		}
	})

	userHandler.Routes(r, authMiddleware)
	siteHandler.Routes(r, authMiddleware)

	return r
}
