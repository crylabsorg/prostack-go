package ps_mongo //nolint:staticcheck // Preserve the public package name for import compatibility.

import (
	"errors"
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

const mongoDuplicateKeyCode = 11000

func IsUniqueConstraintError(err error, namedIndex *NamedIndex) bool {
	if err == nil {
		return false
	}

	if err, ok := errors.AsType[mongo.WriteException](err); ok {
		isOnlyUniqueConstraintError := len(err.WriteErrors) == 1 && err.WriteErrors[0].Code == mongoDuplicateKeyCode
		if !isOnlyUniqueConstraintError {
			return false
		}
		writeErr := err.WriteErrors[0]
		return strings.HasPrefix(
			writeErr.Message,
			fmt.Sprintf(
				"E11000 duplicate key error collection: %s.%s index: %s dup key:",
				namedIndex.Collection.Database().Name(),
				namedIndex.Collection.Name(),
				namedIndex.Name,
			),
		)
	}

	return false
}
