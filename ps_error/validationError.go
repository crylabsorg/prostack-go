package ps_error //nolint:staticcheck // Preserve the public package name for import compatibility.

import (
	"net/http"

	"github.com/go-playground/validator/v10"
	"google.golang.org/grpc/codes"
)

func ValidationErrorToPSError(validationErrors validator.ValidationErrors) PSError {
	args := make([]any, 0, len(validationErrors))
	for _, validationErr := range validationErrors {
		args = append(args, map[string]string{
			"field": validationErr.Field(),
			"error": validationErr.Tag(),
		})
	}
	return PSError{
		ErrorCode:     ErrorCodeValidationFailed,
		Message:       "validation failed",
		Args:          args,
		HStatus:       http.StatusBadRequest,
		GStatus:       codes.InvalidArgument,
		OriginalError: validationErrors,
	}
}
