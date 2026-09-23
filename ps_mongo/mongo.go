package ps_mongo

import (
	"context"
	"errors"
	"time"

	"github.com/crylabsorg/prostack-go/logger"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/connstring"
	"go.opentelemetry.io/contrib/instrumentation/go.mongodb.org/mongo-driver/v2/mongo/otelmongo"

	"go.uber.org/zap"
)

type PSMongo struct {
	Client *mongo.Client
	DB     *mongo.Database
}

func (m *PSMongo) Disconnect(ctx context.Context) {
	if err := m.Client.Disconnect(ctx); err != nil {
		logger.GetLogger(ctx).Error("failed to disconnect from mongo", zap.Error(err))
	}
}

func New(ctx context.Context, uri string) (*mongo.Database, func()) {
	parsedURI, err := connstring.ParseAndValidate(uri)
	if err != nil {
		logger.GetLogger(ctx).Fatal("failed to connect to mongo", zap.Error(err))
	}

	clientOptions := options.Client().ApplyURI(uri).SetMonitor(otelmongo.NewMonitor())
	mongoClient, err := mongo.Connect(clientOptions)
	if err != nil {
		logger.GetLogger(ctx).Fatal("failed to connect to mongo", zap.Error(err))
	}
	logger.GetLogger(ctx).Info("connected to MongoDB")

	const timeout = 10 * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	pingErr := mongoClient.Ping(ctx, readpref.Primary())
	if pingErr != nil {
		disconnectErr := mongoClient.Disconnect(ctx)
		err = errors.Join(pingErr, disconnectErr)
		logger.GetLogger(ctx).Fatal("failed to connect to mongo", zap.Error(err))
	}
	logger.GetLogger(ctx).Info("MongoDB pinged successfully")

	db := mongoClient.Database(parsedURI.Database)

	disconnect := func() {
		const disconnectTimeout = 10 * time.Second
		disconnectCtx, disconnectCancel := context.WithTimeout(context.WithoutCancel(ctx), disconnectTimeout)
		defer disconnectCancel()
		if disconnectErr := mongoClient.Disconnect(disconnectCtx); disconnectErr != nil {
			logger.GetLogger(disconnectCtx).Error("failed to disconnect from mongo", zap.Error(disconnectErr))
		}
	}

	return db, disconnect
}
