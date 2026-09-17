package domain

import "errors"

var (
	ErrNotFound     = errors.New("not found")
	ErrAccessDenied = errors.New("access denied: insufficient permissions")
	ErrUnauthorized = errors.New("unauthorized")

	ErrEmailAlreadyExists    = errors.New("user with this email already exists")
	ErrTelegramAlreadyExists = errors.New("user with this telegram id already exists")
	ErrInvalidCredentials    = errors.New("invalid credentials")
	ErrAdminRoleForbidden    = errors.New("cannot create a user with administrator rights")
	ErrRoleChangeForbidden   = errors.New("only administrator can change user role")
	ErrInvalidRole           = errors.New("invalid user role")
	ErrPasswordPolicy        = errors.New("password does not meet the policy")

	ErrSiteNotBelongUser   = errors.New("site does not belong to user")
	ErrSiteAlreadyVerified = errors.New("site already verified")
	ErrSiteNotVerified     = errors.New("site not verified")
	ErrSiteNotActive       = errors.New("site not active")

	ErrInvalidToken    = errors.New("invalid verification token")
	ErrTokenGeneration = errors.New("failed to generate verification token")
)
