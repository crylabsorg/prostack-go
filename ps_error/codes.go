package ps_error

type ErrorCode int32

const (
	ErrorCodeGeneral ErrorCode = iota
	ErrorCodeValidationFailed
)
