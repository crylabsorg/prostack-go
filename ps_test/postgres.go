package ps_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/crylabsorg/prostack-go/ps_pg"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

const postgresImage = "postgres:18.6-alpine3.24"

func NewPostgresContainer(t *testing.T, schemaPath string) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	container, err := postgres.Run(ctx, postgresImage,
		postgres.WithDatabase("postgres"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		postgres.WithInitScripts(filepath.Clean(schemaPath)),
	)
	require.NoError(t, err)

	databaseURL, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	db, disconnect, err := ps_pg.Connect(ctx, databaseURL)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, container.Terminate(ctx)) })
	t.Cleanup(disconnect)
	return db
}
