//nolint:nolintlint,revive // ignore
package ps_grpc //nolint:staticcheck // Preserve the public package name for import compatibility.

import (
	"context"
	"runtime/debug"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/crylabsorg/prostack-go/logger"
)

func RecoveryInterceptor(
	ctx context.Context,
	req any,
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (any, error) {
	var resp any
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				logger.GetLogger(ctx).Error(
					"recovered from panic in grpc handler",
					zap.String("method", info.FullMethod),
					zap.Any("panic", r),
					zap.ByteString("stack", debug.Stack()),
				)
				err = status.Error(codes.Internal, "internal server error")
			}
		}()
		resp, err = handler(ctx, req)
	}()
	return resp, err
}
