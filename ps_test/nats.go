package ps_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/nats"
)

const natsImage = "nats:2.14.6-alpine3.22"

func NewNATSContainer(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	container, err := nats.Run(ctx, natsImage, testcontainers.WithCmd("-js"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, container.Terminate(ctx)) })
	url, err := container.ConnectionString(ctx)
	require.NoError(t, err)
	return url
}
