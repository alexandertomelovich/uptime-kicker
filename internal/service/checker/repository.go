package service

import "health_checker/internal/repository"

// Проверки соответствия конкретных реализаций интерфейсам checker-сервиса.
// Если сигнатуры разойдутся, ошибка проявится на этапе компиляции.
var (
	_ SiteRepository  = (*repository.SiteRepository)(nil)
	_ CheckRepository = (*repository.CheckRepository)(nil)
)
