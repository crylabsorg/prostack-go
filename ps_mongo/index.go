package ps_mongo //nolint:staticcheck // Preserve the public package name for import compatibility.

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"

	"github.com/crylabsorg/prostack-go/logger"
)

type NamedIndex struct {
	Name       string
	Collection *mongo.Collection
	IndexModel mongo.IndexModel
}

func EnsureIndexes(ctx context.Context, collection *mongo.Collection, desiredIndexes []*NamedIndex) {
	validateDesiredIndexes(ctx, collection, desiredIndexes)
	existingNames := listExistingIndexNames(ctx, collection)
	syncIndexes(ctx, collection, desiredIndexes, existingNames)
	dropStaleIndexes(ctx, collection, desiredIndexes, existingNames)
}

func validateDesiredIndexes(
	ctx context.Context,
	collection *mongo.Collection,
	desiredIndexes []*NamedIndex,
) {
	for i, desiredIndex := range desiredIndexes {
		if desiredIndex.Collection.Name() != collection.Name() {
			logger.GetLogger(ctx).Fatal(
				"index collection does not match the provided collection",
				zap.String("collection", collection.Name()),
				zap.Int("index", i),
				zap.String("indexCollection", desiredIndex.Collection.Name()),
			)
		}
		if desiredIndex.Name == "" {
			logger.GetLogger(ctx).Fatal(
				"index name cannot be empty",
				zap.String("collection", collection.Name()),
				zap.Int("index", i),
			)
		}

		if desiredIndex.IndexModel.Options == nil {
			desiredIndex.IndexModel.Options = options.Index()
		}
		desiredIndex.IndexModel.Options.SetName(desiredIndex.Name)
	}
}

func listExistingIndexNames(
	ctx context.Context,
	collection *mongo.Collection,
) map[string]any {
	existingNames := make(map[string]any)
	existingIndexesList, err := collection.Indexes().ListSpecifications(ctx)
	if err != nil {
		logger.GetLogger(ctx).Fatal(
			"failed to list existing indexes",
			zap.String("collection", collection.Name()),
			zap.Error(err),
		)
	}

	for _, existingIndex := range existingIndexesList {
		existingNames[existingIndex.Name] = nil
	}
	delete(existingNames, "_id_")

	return existingNames
}

func syncIndexes(
	ctx context.Context,
	collection *mongo.Collection,
	desiredIndexes []*NamedIndex,
	existingNames map[string]any,
) {
	for _, desiredIndex := range desiredIndexes {
		if _, exists := existingNames[desiredIndex.Name]; exists {
			logger.GetLogger(ctx).Info(
				"index already exists, skipping",
				zap.String("collection", collection.Name()),
				zap.String("indexName", desiredIndex.Name),
			)
			continue
		}
		_, err := collection.Indexes().CreateOne(ctx, desiredIndex.IndexModel)
		if err != nil {
			logger.GetLogger(ctx).Fatal(
				"failed to create index",
				zap.String("collection", collection.Name()),
				zap.String("index", desiredIndex.Name),
				zap.Error(err),
			)
		}
		logger.GetLogger(ctx).Info(
			"created index",
			zap.String("collection", collection.Name()),
			zap.String("indexName", desiredIndex.Name),
		)
	}
}

func dropStaleIndexes(
	ctx context.Context,
	collection *mongo.Collection,
	desiredIndexes []*NamedIndex,
	existingNames map[string]any,
) {
	indexesToDrop := make(map[string]any)
	for existingIndexName := range existingNames {
		indexesToDrop[existingIndexName] = nil
	}
	for _, desiredIndex := range desiredIndexes {
		delete(indexesToDrop, desiredIndex.Name)
	}

	if len(indexesToDrop) == 0 {
		logger.GetLogger(ctx).Info(
			"no indexes to drop",
			zap.String("collection", collection.Name()),
		)
		return
	}

	for indexName := range indexesToDrop {
		err := collection.Indexes().DropOne(ctx, indexName)
		if err != nil {
			logger.GetLogger(ctx).Fatal(
				"failed to drop index",
				zap.String("collection", collection.Name()),
				zap.String("index", indexName),
				zap.Error(err),
			)
		}
		logger.GetLogger(ctx).Info(
			"dropped index",
			zap.String("collection", collection.Name()),
			zap.String("indexName", indexName),
		)
	}
}
