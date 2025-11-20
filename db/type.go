package db

import (
	"context"

	"github.com/shortlink-org/go-sdk/config"
)

// DB - common interface of db
type DB interface {
	Init(ctx context.Context) error
	GetConn() any
}

// Store abstract type
type Store struct {
	DB

	typeStore string
	cfg       *config.Config
}
