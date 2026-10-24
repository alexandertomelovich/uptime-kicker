package service

import (
	"context"
	"errors"
	"testing"

	"health_checker/internal/domain"

	"github.com/google/uuid"
)

func TestValidateSiteURL(t *testing.T) {
	cases := []struct {
		name string
		url  string
		ok   bool
	}{
		// Допустимые.
		{"https url", "https://example.com", true},
		{"http url", "http://example.com", true},
		{"with path and query", "https://example.com/path?x=1", true},
		{"public ip", "http://8.8.8.8/health", true},
		{"public ip with port", "https://1.1.1.1:8443", true},

		// Некорректные адреса.
		{"empty", "", false},
		{"no host", "https://", false},
		{"not a url", "just text", false},
		{"ftp scheme", "ftp://example.com", false},
		{"file scheme", "file:///etc/passwd", false},

		// Внутренние цели — именно от них защищаемся (SSRF).
		{"localhost", "http://localhost:8080", false},
		{"LOCALHOST uppercase", "http://LOCALHOST", false},
		{"loopback v4", "http://127.0.0.1", false},
		{"loopback v4 subnet", "http://127.8.8.8", false},
		{"loopback v6", "http://[::1]", false},
		{"private 10/8", "http://10.0.0.5", false},
		{"private 172.16/12", "http://172.16.1.1", false},
		{"private 192.168/16", "http://192.168.1.1", false},
		{"cloud metadata", "http://169.254.169.254/latest/meta-data/", false},
		{"unspecified", "http://0.0.0.0", false},
		{"multicast", "http://224.0.0.1", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSiteURL(tc.url)
			if tc.ok && err != nil {
				t.Errorf("url %q: неожиданная ошибка %v", tc.url, err)
			}
			if !tc.ok && !errors.Is(err, domain.ErrInvalidURL) {
				t.Errorf("url %q: err = %v, want ErrInvalidURL", tc.url, err)
			}
		})
	}
}

// Create отклоняет недопустимый URL до обращения к репозиторию.
func TestSiteService_Create_InvalidURL(t *testing.T) {
	repo := &mockSiteRepo{}
	svc := newTestSiteService(repo, nil)

	_, err := svc.Create(context.Background(), CreateSiteRequest{
		Url: "http://169.254.169.254/latest/meta-data/",
	}, uuid.New())
	if !errors.Is(err, domain.ErrInvalidURL) {
		t.Fatalf("err = %v, want ErrInvalidURL", err)
	}
	if repo.lastCreated.Url != "" {
		t.Error("сайт с недопустимым URL не должен попадать в репозиторий")
	}
}

// Update отклоняет смену URL на недопустимый до обращения к репозиторию.
func TestSiteService_Update_InvalidURL(t *testing.T) {
	userID := uuid.New()
	badURL := "http://10.0.0.1/admin"
	getCalled := false
	repo := &mockSiteRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.Site, error) {
			getCalled = true
			return domain.Site{ID: id, UserID: userID}, nil
		},
	}
	svc := newTestSiteService(repo, nil)

	_, err := svc.Update(context.Background(), domain.SiteUpdate{
		ID:     uuid.New(),
		UserID: userID,
		Url:    &badURL,
	})
	if !errors.Is(err, domain.ErrInvalidURL) {
		t.Fatalf("err = %v, want ErrInvalidURL", err)
	}
	if getCalled {
		t.Error("недопустимый URL должен отклоняться до обращения к репозиторию")
	}
}

// Update с корректным URL проходит валидацию.
func TestSiteService_Update_ValidURL(t *testing.T) {
	userID := uuid.New()
	goodURL := "https://example.com/new"
	repo := &mockSiteRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.Site, error) {
			return domain.Site{ID: id, UserID: userID}, nil
		},
	}
	svc := newTestSiteService(repo, nil)

	_, err := svc.Update(context.Background(), domain.SiteUpdate{
		ID:     uuid.New(),
		UserID: userID,
		Url:    &goodURL,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
