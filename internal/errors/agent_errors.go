package errors

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ErrorType categorizes different types of agent communication errors
type ErrorType string

const (
	ErrorTypeNetwork       ErrorType = "network"
	ErrorTypeRateLimit     ErrorType = "rate_limit"
	ErrorTypeAuthentication ErrorType = "auth"
	ErrorTypeTimeout       ErrorType = "timeout"
	ErrorTypeAPI           ErrorType = "api"
	ErrorTypePermanent     ErrorType = "permanent"
	ErrorTypeTemporary     ErrorType = "temporary"
)

// AgentCommunicationError represents a structured error from agent communications
type AgentCommunicationError struct {
	Type          ErrorType
	Message       string
	Retryable     bool
	RetryAfter    time.Duration
	OriginalError error
	Context       map[string]interface{}
}

// Error implements the error interface
func (e *AgentCommunicationError) Error() string {
	if e.OriginalError != nil {
		return fmt.Sprintf("%s: %s (original: %v)", e.Type, e.Message, e.OriginalError)
	}
	return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

// Unwrap returns the underlying error for error chain support
func (e *AgentCommunicationError) Unwrap() error {
	return e.OriginalError
}

// Is implements error identity checking
func (e *AgentCommunicationError) Is(target error) bool {
	if t, ok := target.(*AgentCommunicationError); ok {
		return e.Type == t.Type
	}
	return false
}

// IsRetryable returns whether the error should be retried
func (e *AgentCommunicationError) IsRetryable() bool {
	return e.Retryable
}

// GetRetryAfter returns the suggested delay before retry
func (e *AgentCommunicationError) GetRetryAfter() time.Duration {
	return e.RetryAfter
}

// WithContext adds context information to the error
func (e *AgentCommunicationError) WithContext(key string, value interface{}) *AgentCommunicationError {
	if e.Context == nil {
		e.Context = make(map[string]interface{})
	}
	e.Context[key] = value
	return e
}

// Predefined error instances
var (
	ErrNetworkTimeout = &AgentCommunicationError{
		Type:       ErrorTypeTimeout,
		Message:    "network timeout",
		Retryable:  true,
		RetryAfter: 5 * time.Second,
	}

	ErrRateLimited = &AgentCommunicationError{
		Type:       ErrorTypeRateLimit,
		Message:    "rate limited",
		Retryable:  true,
		RetryAfter: 60 * time.Second,
	}

	ErrAuthentication = &AgentCommunicationError{
		Type:       ErrorTypeAuthentication,
		Message:    "authentication failed",
		Retryable:  false,
		RetryAfter: 0,
	}

	ErrPermanentFailure = &AgentCommunicationError{
		Type:       ErrorTypePermanent,
		Message:    "permanent failure",
		Retryable:  false,
		RetryAfter: 0,
	}
)

// NewNetworkError creates a new network-related error
func NewNetworkError(msg string, err error) *AgentCommunicationError {
	return &AgentCommunicationError{
		Type:          ErrorTypeNetwork,
		Message:       msg,
		Retryable:     true,
		RetryAfter:    5 * time.Second,
		OriginalError: err,
	}
}

// NewRateLimitError creates a new rate limit error
func NewRateLimitError(msg string, retryAfter time.Duration) *AgentCommunicationError {
	return &AgentCommunicationError{
		Type:       ErrorTypeRateLimit,
		Message:    msg,
		Retryable:  true,
		RetryAfter: retryAfter,
	}
}

// NewAuthError creates a new authentication error
func NewAuthError(msg string, err error) *AgentCommunicationError {
	return &AgentCommunicationError{
		Type:          ErrorTypeAuthentication,
		Message:       msg,
		Retryable:     false,
		RetryAfter:    0,
		OriginalError: err,
	}
}

// NewTimeoutError creates a new timeout error
func NewTimeoutError(msg string, err error) *AgentCommunicationError {
	return &AgentCommunicationError{
		Type:          ErrorTypeTimeout,
		Message:       msg,
		Retryable:     true,
		RetryAfter:    10 * time.Second,
		OriginalError: err,
	}
}

// NewAPIError creates a new API error
func NewAPIError(msg string, err error) *AgentCommunicationError {
	return &AgentCommunicationError{
		Type:          ErrorTypeAPI,
		Message:       msg,
		Retryable:     true,
		RetryAfter:    5 * time.Second,
		OriginalError: err,
	}
}

// NewPermanentError creates a new permanent error that should not be retried
func NewPermanentError(msg string, err error) *AgentCommunicationError {
	return &AgentCommunicationError{
		Type:          ErrorTypePermanent,
		Message:       msg,
		Retryable:     false,
		RetryAfter:    0,
		OriginalError: err,
	}
}

// ClassifyError attempts to classify a generic error into a structured AgentCommunicationError
func ClassifyError(err error) *AgentCommunicationError {
	if err == nil {
		return nil
	}

	// If it's already our error type, return it
	var agentErr *AgentCommunicationError
	if errors.As(err, &agentErr) {
		return agentErr
	}

	// Check for common HTTP status codes if available
	if httpErr, ok := err.(interface {
		StatusCode() int
	}); ok {
		switch httpErr.StatusCode() {
		case http.StatusTooManyRequests:
			return NewRateLimitError("HTTP 429: Too Many Requests", 60*time.Second)
		case http.StatusUnauthorized:
			return NewAuthError("HTTP 401: Unauthorized", err)
		case http.StatusForbidden:
			return NewAuthError("HTTP 403: Forbidden", err)
		case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return NewAPIError(fmt.Sprintf("HTTP %d: Server Error", httpErr.StatusCode()), err)
		case http.StatusNotFound, http.StatusBadRequest:
			return NewPermanentError(fmt.Sprintf("HTTP %d: Client Error", httpErr.StatusCode()), err)
		}
	}

	// Check for timeout errors
	if isTimeoutError(err) {
		return NewTimeoutError("operation timed out", err)
	}

	// Check for network errors
	if isNetworkError(err) {
		return NewNetworkError("network error", err)
	}

	// Default to temporary error for unknown cases
	return &AgentCommunicationError{
		Type:          ErrorTypeTemporary,
		Message:       "unknown temporary error",
		Retryable:     true,
		RetryAfter:    5 * time.Second,
		OriginalError: err,
	}
}

// isTimeoutError checks if an error is a timeout error
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}

	// Check for context deadline exceeded
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	// Check for net.Error timeout
	if netErr, ok := err.(interface {
		Timeout() bool
	}); ok && netErr.Timeout() {
		return true
	}

	// Check error message for timeout keywords
	msg := err.Error()
	return contains(msg, "timeout") || contains(msg, "deadline exceeded")
}

// isNetworkError checks if an error is a network-related error
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}

	msg := err.Error()
	return contains(msg, "connection refused") ||
		contains(msg, "no such host") ||
		contains(msg, "network unreachable") ||
		contains(msg, "connection reset")
}

// contains checks if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && 
		(s == substr || (len(s) > len(substr) && 
			(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || 
				strings.Contains(strings.ToLower(s), strings.ToLower(substr)))))
}