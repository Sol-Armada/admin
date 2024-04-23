package utils

import (
	"context"
)

type ContextKey string

const (
	MEMBER ContextKey = "member"
	LOGGER ContextKey = "logger"
)

func SetMemberToContext(ctx context.Context, member any) context.Context {
	return context.WithValue(ctx, MEMBER, member)
}

func GetMemberFromContext(ctx context.Context) any {
	return ctx.Value(MEMBER)
}

func SetLoggerToContext(ctx context.Context, logger any) context.Context {
	return context.WithValue(ctx, LOGGER, logger)
}

func GetLoggerFromContext(ctx context.Context) any {
	return ctx.Value(LOGGER)
}
