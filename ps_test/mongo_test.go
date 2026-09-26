package ps_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

//nolint:gochecknoglobals // fine
var mongoContainer MongoContainer

func TestMain(m *testing.M) {
	var cleanup func()
	mongoContainer, cleanup = NewMongoContainerForM()
	ctx := context.Background()
	if err := mongoContainer.DB.Client().Ping(ctx, nil); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to ping mongo testcontainer: %v\n", err)
		cleanup()
		os.Exit(1)
	}
	code := m.Run()
	cleanup()
	os.Exit(code)
}

func TestIntegration_StartMongoEnv(t *testing.T) {
	mongoContainer1 := NewMongoContainer(t)
	client := mongoContainer1.DB.Client()
	ctx := context.Background()
	require.NoError(t, client.Ping(ctx, nil))
	require.NoError(t, mongoContainer1.DB.Client().Ping(ctx, nil))
}
