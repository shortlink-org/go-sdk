package mongo

import (
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/shortlink-org/go-sdk/config"
)

// Config - config
type Config struct {
	URI  string
	mode int
}

// Store implementation of db interface
type Store struct {
	client *mongo.Client
	config Config
	cfg    *config.Config
}
