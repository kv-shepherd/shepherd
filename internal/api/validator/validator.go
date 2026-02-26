package validator

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	govalidator "github.com/go-playground/validator/v10"

	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
)

var (
	validateOnce sync.Once
	validateInst *govalidator.Validate
)

// ValidateStruct validates a request struct using a singleton validator engine.
// It returns AppError with field-level details for request validation failures.
func ValidateStruct(payload any) error {
	err := getValidator().Struct(payload)
	if err == nil {
		return nil
	}

	var ve govalidator.ValidationErrors
	if errors.As(err, &ve) {
		fieldErrors := make([]apperrors.FieldError, 0, len(ve))
		for _, fe := range ve {
			field := strings.TrimSpace(fe.Field())
			if field == "" {
				field = fe.StructField()
			}
			fieldErrors = append(fieldErrors, apperrors.FieldError{
				Field:   field,
				Code:    validationCode(fe.Tag()),
				Message: validationMessage(fe),
			})
		}

		return apperrors.BadRequest("INVALID_REQUEST", "request validation failed").WithFieldErrors(fieldErrors)
	}

	var invalidErr *govalidator.InvalidValidationError
	if errors.As(err, &invalidErr) {
		return apperrors.Internal("INTERNAL_ERROR", "validator received invalid input")
	}

	return apperrors.BadRequest("INVALID_REQUEST", "request validation failed")
}

func getValidator() *govalidator.Validate {
	validateOnce.Do(func() {
		v := govalidator.New(govalidator.WithRequiredStructEnabled())
		v.RegisterTagNameFunc(func(field reflect.StructField) string {
			jsonTag := field.Tag.Get("json")
			if jsonTag == "" {
				return field.Name
			}

			tagName := strings.TrimSpace(strings.Split(jsonTag, ",")[0])
			if tagName == "" {
				return field.Name
			}
			if tagName == "-" {
				return ""
			}
			return tagName
		})
		validateInst = v
	})

	return validateInst
}

func validationCode(tag string) string {
	cleaned := strings.TrimSpace(tag)
	if cleaned == "" {
		return "VALIDATION_FAILED"
	}
	cleaned = strings.ReplaceAll(cleaned, "-", "_")
	return strings.ToUpper(cleaned)
}

func validationMessage(fe govalidator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "is required"
	case "min", "gte":
		return fmt.Sprintf("must be >= %s", fe.Param())
	case "max", "lte":
		return fmt.Sprintf("must be <= %s", fe.Param())
	case "oneof":
		return fmt.Sprintf("must be one of [%s]", fe.Param())
	default:
		if p := strings.TrimSpace(fe.Param()); p != "" {
			return fmt.Sprintf("failed validation '%s=%s'", fe.Tag(), p)
		}
		return fmt.Sprintf("failed validation '%s'", fe.Tag())
	}
}
