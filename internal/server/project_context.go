package server

import "context"

type projectContextKey struct{}

func contextWithProject(ctx context.Context, project string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, projectContextKey{}, project)
}

func projectFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	value, ok := ctx.Value(projectContextKey{}).(string)
	if !ok {
		return "", false
	}
	value, err := normalizePrefix(value)
	if err != nil {
		return "", false
	}
	return value, true
}
