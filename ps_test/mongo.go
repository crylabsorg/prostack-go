package ps_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/crylabsorg/prostack-go/ps_mongo"
)

const mongoImage = "mongo:8.0.6"

type MongoContainer struct {
	Container testcontainers.Container
	DB        *mongo.Database
}

func NewMongoContainer(t *testing.T) MongoContainer {
	t.Helper()

	ctx := context.Background()

	container, err := mongodb.Run(
		ctx,
		mongoImage,
		mongodb.WithReplicaSet("rs0"),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, container.Terminate(ctx))
	})

	uri, err := container.ConnectionString(ctx)
	require.NoError(t, err)

	parsedURI, err := url.Parse(uri)
	require.NoError(t, err)
	require.Equal(t, "/", parsedURI.Path)
	q := parsedURI.Query()
	q.Set("directConnection", "true")
	parsedURI.RawQuery = q.Encode()
	parsedURI.Path = "testdb"

	db, mongoDisconnect := ps_mongo.New(ctx, parsedURI.String())
	t.Cleanup(func() {
		mongoDisconnect()
	})

	return MongoContainer{
		Container: container,
		DB:        db,
	}
}

func NewMongoContainerForM() (MongoContainer, func()) {
	ctx := context.Background()

	container, err := mongodb.Run(
		ctx,
		mongoImage,
		mongodb.WithReplicaSet("rs0"),
	)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to initialize mongo testcontainer: %v\n", err)
		os.Exit(1)
	}

	uri, err := container.ConnectionString(ctx)
	if err != nil {
		terminationErr := container.Terminate(ctx)
		_, _ = fmt.Fprintf(
			os.Stderr,
			"failed to initialize mongo testcontainer: %v\n",
			errors.Join(err, terminationErr),
		)
		os.Exit(1)
	}

	parsedURI, err := url.Parse(uri)
	if err != nil {
		terminationErr := container.Terminate(ctx)
		_, _ = fmt.Fprintf(
			os.Stderr,
			"failed to initialize mongo testcontainer: %v\n",
			errors.Join(err, terminationErr),
		)
		os.Exit(1)
	}
	if parsedURI.Path != "/" {
		unexpectedPathErr := errors.New("unexpected path in mongo URI")
		terminationErr := container.Terminate(ctx)
		_, _ = fmt.Fprintf(
			os.Stderr,
			"failed to initialize mongo testcontainer: %v\n",
			errors.Join(unexpectedPathErr, terminationErr),
		)
		os.Exit(1)
	}
	q := parsedURI.Query()
	q.Set("directConnection", "true")
	parsedURI.RawQuery = q.Encode()
	parsedURI.Path = "testdb"

	db, mongoDisconnect := ps_mongo.New(ctx, parsedURI.String())
	cleanup := func() {
		mongoDisconnect()
		err1 := container.Terminate(ctx)
		if err1 != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to cleanup mongo testcontainer: %v\n", err1)
			os.Exit(1)
		}
	}

	mc := MongoContainer{
		Container: container,
		DB:        db,
	}
	return mc, cleanup
}
