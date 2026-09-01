package kratos

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	ory "github.com/ory/client-go"
	"github.com/spf13/viper"

	"github.com/shortlink-org/go-sdk/config"
)

// Client wraps Ory Kratos Admin API client for getting user email by user ID
type Client struct {
	client *ory.APIClient
	log    *slog.Logger
}

// New creates a new Kratos Admin API client
func New(log *slog.Logger, cfg *config.Config) (*Client, error) {
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
	if resp != nil && resp.Body != nil {
		// The generated Ory client hands back the live response; without this
		// the connection is never returned to the pool.
		defer func() {
			errClose := resp.Body.Close()
			if errClose != nil {
				c.log.WarnContext(ctx, "failed to close Kratos response body",
					slog.String("user_id", userID),
					slog.String("error", errClose.Error()),
				)
			}
		}()
	}

	if err != nil {
		c.log.ErrorContext(ctx, "failed to get identity from Kratos",
			slog.String("user_id", userID),
			slog.String("error", err.Error()),
		)

		// According to ADR-42: any error should be treated as permission denied
		// to avoid revealing information about user existence
		return "", fmt.Errorf("failed to get user identity: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		c.log.ErrorContext(ctx, "unexpected status code from Kratos",
			slog.String("user_id", userID),
			slog.Int("status_code", resp.StatusCode),
		)

		return "", fmt.Errorf("%w: %d", errUnexpectedStatus, resp.StatusCode)
	}

	// Extract email from identity traits
	// According to ADR-42, email is stored in identity.Traits["email"]
	// Traits is of type map[string]any
	traits, isMap := identity.Traits.(map[string]any)
	if !isMap || traits == nil {
		c.log.ErrorContext(ctx, "identity traits is not a valid map or nil",
			slog.String("user_id", userID),
		)

		return "", errTraitsNotMap
	}

	emailInterface, exists := traits["email"]
	if !exists {
		c.log.ErrorContext(ctx, "email not found in identity traits",
			slog.String("user_id", userID),
		)

		return "", errEmailMissing
	}

	email, ok := emailInterface.(string)
	if !ok || email == "" {
		c.log.ErrorContext(ctx, "email is not a valid string or empty",
			slog.String("user_id", userID),
		)

		return "", errEmailNotString
	}

	return email, nil
}
