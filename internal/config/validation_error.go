package config

import (
	"fmt"
	"strings"
)

// ValidationError provides structured error reporting with actionable feedback.
type ValidationError struct {
	Field   string
	Value   interface{}
	Rule    string
	Message string
}

func (v *ValidationError) Error() string {
	return fmt.Sprintf("config.%s: %s (got: %v)", v.Field, v.Message, v.Value)
}

// ValidationErrors aggregates multiple validation errors.
type ValidationErrors struct {
	Errors []ValidationError
}

func (ve *ValidationErrors) Error() string {
	if len(ve.Errors) == 0 {
		return "no validation errors"
	}
	
	if len(ve.Errors) == 1 {
		return ve.Errors[0].Error()
	}
	
	var messages []string
	messages = append(messages, fmt.Sprintf("found %d validation errors:", len(ve.Errors)))
	for _, err := range ve.Errors {
		messages = append(messages, "  - "+err.Error())
	}
	
	return strings.Join(messages, "\n")
}

// Add appends a validation error to the collection.
func (ve *ValidationErrors) Add(field string, value interface{}, rule string, message string) {
	ve.Errors = append(ve.Errors, ValidationError{
		Field:   field,
		Value:   value,
		Rule:    rule,
		Message: message,
	})
}

// HasErrors returns true if there are validation errors.
func (ve *ValidationErrors) HasErrors() bool {
	return len(ve.Errors) > 0
}

// ToError returns the ValidationErrors as an error interface, or nil if no errors.
func (ve *ValidationErrors) ToError() error {
	if !ve.HasErrors() {
		return nil
	}
	return ve
}