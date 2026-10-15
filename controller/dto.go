package controller

import (
	"encoding/json/jsontext"
)

// ErrorResponse is the body returned for every non-2xx response.
type ErrorResponse struct {
	// Message is a human-readable description of what went wrong. Intended for
	// logging and debugging, not for programmatic branching (use errorCode for that).
	Message string `json:"message"`
	// ErrorCode is a stable numeric application error code identifying the error type.
	// Use it for programmatic handling; it does not change across locales or message wording.
	ErrorCode int `json:"errorCode"`
	// Args are optional values interpolated into the message template (e.g. field names
	// or limits). Their order matches the placeholders in the message.
	Args []jsontext.Value `json:"args,omitempty"`
}
