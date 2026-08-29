//go:build unit

package postgres

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrimaryApplicationNameDoesNotLeakIntoReplicaBase(t *testing.T) {
	t.Parallel()

	config, err := pgxpool.ParseConfig("postgres://localhost/db?application_name=shortlink")
	require.NoError(t, err)

	replicaConfig := config.Copy()
	applicationName(config, applicationNamePrimary)

	assert.Equal(t, "shortlink-primary", config.ConnConfig.RuntimeParams["application_name"])
	assert.Equal(t, "shortlink", replicaConfig.ConnConfig.RuntimeParams["application_name"])
}
