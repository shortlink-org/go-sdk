//go:build unit

package db

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/shortlink-org/go-sdk/config"
)

func TestLink(t *testing.T) {
	ctx := context.Background()

	// Init logger
	log := slog.New(slog.DiscardHandler)

	cfg, err := config.New()
	require.NoError(t, err, "Error init config")

	// Init db
	s, err := New(ctx, log, nil, nil, cfg)
	require.NoError(t, err, "Error init a db")

	// Init db
	require.NoError(t, s.Init(ctx), "Error  create a new DB connection")
}
