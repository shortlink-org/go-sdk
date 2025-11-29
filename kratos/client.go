package kratos

import (
	"context"
	"fmt"
	"log/slog"

	ory "github.com/ory/client-go"
	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/logger"
	"github.com/spf13/viper"
)

// Client wraps Ory Kratos Admin API client for getting user email by user ID
type Client struct {
	client *ory.APIClient
	log    logger.Logger
}

// New creates a new Kratos Admin API client
func New(log logger.Logger, cfg *config.Config) (*Client, error) {
	viper.AutomaticEnv()
	kratosAdminURL := viper.GetString("KRATOS_ADMIN_URL")
	if kratosAdminURL == "" {
		kratosAdminURL = "http://kratos:4434" // default Kratos Admin API URL
	}

	configuration := ory.NewConfiguration()
	configuration.Servers = []ory.ServerConfiguration{
		{
			URL: kratosAdminURL,
		},
	}

	apiClient := ory.NewAPIClient(configuration)

	return &Client{
		client: apiClient,
		log:    log,
	}, nil
}

// GetUserEmail retrieves user email by user ID from Kratos Admin API
// Returns email and error. If user not found or email is missing, returns error.
func (c *Client) GetUserEmail(ctx context.Context, userID string) (string, error) {
	identity, resp, err := c.client.IdentityAPI.GetIdentity(ctx, userID).Execute()
	if err != nil {
		c.log.ErrorWithContext(ctx, "failed to get identity from Kratos",
			slog.String("user_id", userID),
			slog.String("error", err.Error()),
		)

		// According to ADR-42: any error should be treated as permission denied
		// to avoid revealing information about user existence
		return "", fmt.Errorf("failed to get user identity: %w", err)
	}

	if resp.StatusCode != 200 {
		c.log.ErrorWithContext(ctx, "unexpected status code from Kratos",
			slog.String("user_id", userID),
			slog.Int("status_code", resp.StatusCode),
		)

		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Extract email from identity traits
	// According to ADR-42, email is stored in identity.Traits["email"]
	// Traits is of type map[string]interface{}
	traits, ok := identity.Traits.(map[string]interface{})
	if !ok || traits == nil {
		c.log.ErrorWithContext(ctx, "identity traits is not a valid map or nil",
			slog.String("user_id", userID),
		)

		return "", fmt.Errorf("identity traits is not a valid map")
	}

	emailInterface, exists := traits["email"]
	if !exists {
		c.log.ErrorWithContext(ctx, "email not found in identity traits",
			slog.String("user_id", userID),
		)

		return "", fmt.Errorf("email not found in identity traits")
	}

	email, ok := emailInterface.(string)
	if !ok || email == "" {
		c.log.ErrorWithContext(ctx, "email is not a valid string or empty",
			slog.String("user_id", userID),
		)

		return "", fmt.Errorf("email is not a valid string")
	}

	return email, nil
}

