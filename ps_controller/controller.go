package ps_controller

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"net/http"

	"go.uber.org/zap"
	"google.golang.org/grpc/status"

	"github.com/crylabsorg/prostack-go/logger"
	errorv1 "github.com/crylabsorg/prostack-go/pkg/proto/prostack/error/v1"
)

type Controller struct{}

func NewController() Controller {
	return Controller{}
}

// writeJSON is the package-level response helper; the Controller methods
// delegate to it so that non-Controller code (e.g. internal/access) can
// write responses without holding a Controller instance.
func writeJSON(ctx context.Context, w http.ResponseWriter, httpStatus int, data any) {
	body, err := json.Marshal(data)
	if err != nil {
		logger.GetLogger(ctx).Warn("failed to encode response", zap.Error(err))
		WriteText(ctx, w, http.StatusInternalServerError, "internal server error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	if _, writeErr := w.Write(body); writeErr != nil {
		logger.GetLogger(ctx).Warn("failed to write response", zap.Error(writeErr))
	}
}

// WriteText writes an error message as a JSON ErrorResponse. It is exported
// so that packages that import controller (e.g. internal/access) can write
// auth errors centrally without a Controller instance.
func WriteText(ctx context.Context, w http.ResponseWriter, httpStatus int, message string) {
	const errCode = 1
	writeJSON(ctx, w, httpStatus, ErrorResponse{Message: message, ErrorCode: errCode})
}

func (c Controller) WriteJSON(ctx context.Context, w http.ResponseWriter, httpStatus int, data any) {
	writeJSON(ctx, w, httpStatus, data)
}

func (c Controller) WriteText(ctx context.Context, w http.ResponseWriter, httpStatus int, message string) {
	WriteText(ctx, w, httpStatus, message)
}

func (c Controller) WriteError(ctx context.Context, w http.ResponseWriter, err error) {
	if grpcErr, ok := status.FromError(err); ok {
		for _, detail := range grpcErr.Details() {
			if typedDetail, ok1 := detail.(*errorv1.Error); ok1 {
				values := typedDetail.GetArgs().GetValues()
				args := make([]jsontext.Value, len(values))
				for i, value := range values {
					data, marshalErr := json.Marshal(value.AsInterface())
					if marshalErr != nil {
						logger.GetLogger(ctx).Warn("failed to encode error argument", zap.Int("index", i), zap.Error(marshalErr))
						WriteText(ctx, w, http.StatusInternalServerError, "internal server error")
						return
					}
					args[i] = jsontext.Value(data)
				}

				writeJSON(ctx, w, int(typedDetail.GetHttpStatus()), ErrorResponse{
					Message:   grpcErr.Message(),
					ErrorCode: int(typedDetail.GetErrorCode()),
					Args:      args,
				})
				return
			}
		}
	}
	WriteText(ctx, w, http.StatusInternalServerError, err.Error())
}
