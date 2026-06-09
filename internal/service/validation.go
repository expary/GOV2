package service

import (
	"errors"
	"strings"
)

type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type ValidationError struct {
	Fields []FieldError `json:"fields"`
}

func (e ValidationError) Error() string {
	return ErrInvalidInput.Error()
}

func (e ValidationError) Unwrap() error {
	return ErrInvalidInput
}

func NewValidationError(fields ...FieldError) error {
	cleaned := make([]FieldError, 0, len(fields))
	for _, field := range fields {
		field.Field = strings.TrimSpace(field.Field)
		field.Message = strings.TrimSpace(field.Message)
		if field.Field == "" {
			continue
		}
		if field.Message == "" {
			field.Message = "Invalid value"
		}
		cleaned = append(cleaned, field)
	}
	if len(cleaned) == 0 {
		return ErrInvalidInput
	}
	return ValidationError{Fields: cleaned}
}

func ValidationFields(err error) ([]FieldError, bool) {
	var validation ValidationError
	if !errors.As(err, &validation) || len(validation.Fields) == 0 {
		return nil, false
	}
	return append([]FieldError(nil), validation.Fields...), true
}
