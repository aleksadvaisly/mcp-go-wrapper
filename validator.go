package mcpwrapper

import (
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
)

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	var messages []string
	for _, err := range e {
		messages = append(messages, err.Error())
	}
	return fmt.Sprintf("validation failed: %s", strings.Join(messages, "; "))
}

func formatValidationErrors(err error) error {
	validationErrs, ok := err.(validator.ValidationErrors)
	if !ok {
		return err
	}

	var errors ValidationErrors
	for _, fieldErr := range validationErrs {
		errors = append(errors, ValidationError{
			Field:   fieldErr.Field(),
			Message: formatFieldError(fieldErr),
		})
	}

	return errors
}

func formatFieldError(fieldErr validator.FieldError) string {
	switch fieldErr.Tag() {
	case "required":
		return "is required"
	case "min":
		return fmt.Sprintf("must be at least %s", fieldErr.Param())
	case "max":
		return fmt.Sprintf("must be at most %s", fieldErr.Param())
	case "email":
		return "must be a valid email address"
	case "url":
		return "must be a valid URL"
	case "oneof":
		return fmt.Sprintf("must be one of: %s", fieldErr.Param())
	case "gte":
		return fmt.Sprintf("must be greater than or equal to %s", fieldErr.Param())
	case "lte":
		return fmt.Sprintf("must be less than or equal to %s", fieldErr.Param())
	case "gt":
		return fmt.Sprintf("must be greater than %s", fieldErr.Param())
	case "lt":
		return fmt.Sprintf("must be less than %s", fieldErr.Param())
	case "len":
		return fmt.Sprintf("must be %s characters long", fieldErr.Param())
	default:
		return fmt.Sprintf("failed validation: %s", fieldErr.Tag())
	}
}
