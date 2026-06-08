package openlist

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
)

// Sentinel errors for error classification.
var (
	ErrNotFound   = errors.New("not found")
	ErrAuth       = errors.New("authentication required")
	ErrForbidden  = errors.New("forbidden")
	ErrRateLimit  = errors.New("rate limited")
	ErrServer     = errors.New("server error")
	ErrEncode     = errors.New("encode error")
	ErrDecode     = errors.New("decode error")
	ErrNetwork    = errors.New("network error")
	ErrUnknown    = errors.New("unknown error")
)

// APIError represents an error returned by the OpenList API.
type APIError struct {
	Code    int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("openlist api error: code=%d message=%s", e.Code, e.Message)
}

// classifyHTTPError maps HTTP status codes to sentinel errors.
func classifyHTTPError(statusCode int, body []byte) error {
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		msg = http.StatusText(statusCode)
	}

	switch statusCode {
	case http.StatusUnauthorized:
		return &classifiedError{ErrAuth, statusCode, msg}
	case http.StatusForbidden:
		return &classifiedError{ErrForbidden, statusCode, msg}
	case http.StatusNotFound:
		return &classifiedError{ErrNotFound, statusCode, msg}
	case http.StatusTooManyRequests:
		return &classifiedError{ErrRateLimit, statusCode, msg}
	case http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return &classifiedError{ErrServer, statusCode, msg}
	default:
		if statusCode >= 500 {
			return &classifiedError{ErrServer, statusCode, msg}
		}
		return &classifiedError{ErrUnknown, statusCode, msg}
	}
}

// classifyNetworkError wraps network-level errors.
func classifyNetworkError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return err
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return &classifiedError{ErrNetwork, 0, netErr.Error()}
	}
	return &classifiedError{ErrNetwork, 0, err.Error()}
}

// isRetryable returns true if the error is safe to retry.
func isRetryable(err error) bool {
	if errors.Is(err, ErrServer) || errors.Is(err, ErrNetwork) || errors.Is(err, ErrRateLimit) {
		return true
	}
	var ce *classifiedError
	if errors.As(err, &ce) {
		return ce.statusCode == http.StatusTooManyRequests ||
			ce.statusCode == http.StatusRequestTimeout ||
			(ce.statusCode >= 500 && ce.statusCode < 600)
	}
	return false
}

// classifiedError pairs a sentinel error with an HTTP status code and message.
type classifiedError struct {
	err        error
	statusCode int
	message    string
}

func (e *classifiedError) Error() string {
	return fmt.Sprintf("%s: %s", e.err.Error(), e.message)
}

func (e *classifiedError) Unwrap() error {
	return e.err
}
