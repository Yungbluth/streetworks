package api

import (
	"context"
	"log/slog"

	"github.com/yungbluth/streetworks/internal/store"
)

const (
	maxLookupIDs = 2000
	defaultLimit = 20
	maxLimit     = 50
)

type Store interface {
	Meta(ctx context.Context) (store.Meta, error)
	Lookup(ctx context.Context, ids []int) (map[int]store.Detail, error)
	ByNumber(ctx context.Context, number string) (store.Permit, error)
	Search(ctx context.Context, q string, limit int) ([]store.Hit, error)
}

type Options struct {
	// Origin is the one site allowed to call the API from a browser
	Origin string
	Log    *slog.Logger
}

type server struct {
	store Store
	built string
	log   *slog.Logger
}
