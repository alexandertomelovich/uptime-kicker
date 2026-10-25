package service

import (
	"net"
	"net/url"
	"strings"

	"health_checker/internal/domain"
)

// validateSiteURL проверяет URL, который пользователь хочет поставить на
// мониторинг. Чекер будет делать к нему HTTP-запросы с сервера, поэтому без
// этой проверки сервис превращается в SSRF-зонд: можно «мониторить»
// localhost, внутренние адреса сети или метаданные облака (169.254.169.254)
// и получать код ответа и время отклика.
//
// Проверка намеренно простая и выполняется только по строке URL (без
// DNS-резолва): hostname «localhost», литеральные IP-адреса из приватных и
// служебных диапазонов запрещаются сразу.
func validateSiteURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return domain.ErrInvalidURL
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return domain.ErrInvalidURL
	}

	host := u.Hostname()
	if host == "" {
		return domain.ErrInvalidURL
	}

	if strings.EqualFold(host, "localhost") {
		return domain.ErrInvalidURL
	}

	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
			ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
			return domain.ErrInvalidURL
		}
	}

	return nil
}
