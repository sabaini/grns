package server

import (
	"context"

	"grns/internal/store"
)

type authContextKey struct{}

type authPrincipal struct {
	AuthType string
	User     *store.AuthUser
}

func contextWithAuthPrincipal(ctx context.Context, principal authPrincipal) context.Context {
	return context.WithValue(ctx, authContextKey{}, principal)
}

func authPrincipalFromContext(ctx context.Context) (authPrincipal, bool) {
	if ctx == nil {
		return authPrincipal{}, false
	}
	principal, ok := ctx.Value(authContextKey{}).(authPrincipal)
	return principal, ok
}
