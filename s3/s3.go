package s3

import (
	"context"
	"log/slog"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/logger"
)

type Client struct {
	client *minio.Client
}

// New creates a new S3 client
func New(ctx context.Context, log logger.Logger, cfg *config.Config) (*Client, error) {
	cfg.SetDefault("S3_ENDPOINT", "localhost:9000")
	cfg.SetDefault("S3_ACCESS_KEY_ID", "minio_access_key")
	cfg.SetDefault("S3_SECRET_ACCESS_KEY", "minio_secret_key")
	cfg.SetDefault("S3_USE_SSL", false)

	client, err := minio.New(cfg.GetString("S3_ENDPOINT"), &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.GetString("S3_ACCESS_KEY_ID"), cfg.GetString("S3_SECRET_ACCESS_KEY"), ""),
		Secure: cfg.GetBool("S3_USE_SSL"),
	})
	if err != nil {
		return nil, err
	}

	if client.IsOffline() {
		return nil, ErrConnectionFailed
	}

	log.Info("S3 client created",
		slog.String("endpoint", cfg.GetString("S3_ENDPOINT")),
	)

	return &Client{
		client: client,
	}, nil
}
