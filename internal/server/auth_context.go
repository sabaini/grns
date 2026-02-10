package server

import (
	"context"

	"grns/internal/store"
)

type authPrincipalContextKey struct{}
type authRequiredContextKey struct{}

type authPrincipal struct {
	AuthType string
	User     *store.AuthUser
}

func contextWithAuthPrincipal(ctx context.Context, principal authPrincipal) context.Context {
	return context.WithValue(ctx, authPrincipalContextKey{}, principal)
}

func authPrincipalFromContext(ctx context.Context) (authPrincipal, bool) {
	if ctx == nil {
		return authPrincipal{}, false
	}
	principal, ok := ctx.Value(authPrincipalContextKey{}).(authPrincipal)
	return principal, ok
}

func contextWithAuthRequired(ctx context.Context, required bool) context.Context {
	return context.WithValue(ctx, authRequiredContextKey{}, required)
}

func authRequiredFromContext(ctx context.Context) (bool, bool) {
	if ctx == nil {
		return false, false
	}
	required, ok := ctx.Value(authRequiredContextKey{}).(bool)
	return required, ok
}
