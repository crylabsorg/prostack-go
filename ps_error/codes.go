package ps_error //nolint:staticcheck // Preserve the public package name for import compatibility.

type ErrorCode int32

const (
	ErrorCodeGeneral ErrorCode = iota
	ErrorCodeValidationFailed
)
