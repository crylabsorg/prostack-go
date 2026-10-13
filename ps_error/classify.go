package ps_error //nolint:staticcheck // Preserve the public package name for import compatibility.

import (
	"google.golang.org/grpc/status"

	pbError "github.com/crylabsorg/prostack-go/pkg/proto/prostack/error/v1"
)

func IsErrorCode(err error, code ErrorCode) bool {
	if err == nil {
		return false
	}
	st, ok := status.FromError(err)
	if !ok {
		return false
	}
	for _, detail := range st.Details() {
		if d, isPBError := detail.(*pbError.Error); isPBError {
			if ErrorCode(d.GetErrorCode()) == code {
				return true
			}
		}
	}
	return false
}
