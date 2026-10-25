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

	// Два клиента Telegram API с разными таймаутами: getUpdates держит
	// соединение до 30 секунд (long-polling), а отправка алертов и реплаев
	// не должна занимать столько же — иначе зависший Telegram API
	// останавливает обработку результатов проверок. Дефолтный клиент
	// tgbotapi.NewBotAPI вообще без таймаута, поэтому задаём явно.
	pollBot, err := tgbotapi.NewBotAPIWithClient(
		cfg.Telegram.Token, tgbotapi.APIEndpoint,
		&http.Client{Timeout: 45 * time.Second},
	)
	if err != nil {
		return err
	}
	sendBot, err := tgbotapi.NewBotAPIWithClient(
		cfg.Telegram.Token, tgbotapi.APIEndpoint,
		&http.Client{Timeout: 10 * time.Second},
	)
	if err != nil {
		return err
	}
	sender := telegram.NewSender(sendBot)
	log.Printf("telegram bot authorized as @%s", pollBot.Self.UserName)

	checker := checkersvc.NewCheckerService(
		siteRepo,
		checkRepo,
		sender,
		cfg.Checker.NumWorkers,
		cfg.Checker.Limit,
		cfg.Checker.QueueSize,
	)
	checker.Start()

	maintenance := cronsvc.NewMaintenanceService(checkRepo, refreshTokenRepo)
	if err := maintenance.StartCron(); err != nil {
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
	telegramHandler := telegram.NewHandler(pollBot, userRepo, siteService)
	telegramBot := telegram.NewBot(pollBot, telegramHandler)

	router := newRouter(userHandler, siteHandler, jwtManager.AuthMiddleware, pool.Ping)
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
	maintenance.StopCron(shutdownCtx)

	log.Println("shutdown complete")
	return nil
}

// newRouter собирает HTTP-роутер. ping используется healthz-пробой:
// живой процесс с мёртвой БД не должен считаться здоровым (балансировщик
// по 503 уведёт трафик на живые инстансы).
func newRouter(
	userHandler *httphandler.UserHandler,
	siteHandler *httphandler.SiteHandler,
	authMiddleware func(http.Handler) http.Handler,
	ping func(ctx context.Context) error,
) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := ping(ctx); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("ok")); err != nil {
			log.Printf("healthz: failed to write response: %v", err)
		}
	})

	userHandler.Routes(r, authMiddleware)
	siteHandler.Routes(r, authMiddleware)

	return r
}
