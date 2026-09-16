package domain

import "errors"

var (
	ErrNotFound                 = errors.New("not found")
	ErrEmailAlreadyExists       = errors.New("user with this email already exists")
	ErrInvalidVerificationToken = errors.New("invalid verification token")
	ErrSiteNotFound             = errors.New("site not found")
	ErrSiteNotBelongUser   = errors.New("site does not belong to user")
	ErrInvalidToken        = errors.New("invalid verification token")
	ErrSiteAlreadyVerified = errors.New("site already verified")
	ErrSiteNotVerified     = errors.New("site not verified")
	ErrSiteNotActive       = errors.New("site not active")
	ErrTokenGeneration     = errors.New("failed to generate verification token")
	ErrAccessDenied        = errors.New("access denied: insufficient permissions")
)
