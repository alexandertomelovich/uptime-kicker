package telegram

import (
	"health_checker/internal/repository"
	"health_checker/internal/service"
)

// Проверки соответствия конкретных реализаций интерфейсам бота.
// Если сигнатуры разойдутся — ошибка на этапе компиляции.
var (
	_ UserProvider = (*repository.UserRepository)(nil)
	_ SiteService  = (*service.SiteService)(nil)
)
