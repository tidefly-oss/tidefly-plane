package middleware

// Mit Huma v2 ist dieser File weitgehend obsolet.
//
// Huma validiert Request-Parameter und Bodies automatisch aus den
// Input-Struct-Tags:
//
//   type CreateUserInput struct {
//       Body struct {
//           Email string `json:"email" format:"email" doc:"User email"`
//           Role  string `json:"role"  enum:"admin,member" doc:"User role"`
//           Name  string `json:"name"  minLength:"2" maxLength:"100"`
//       }
//   }
//
// Bei Validierungsfehlern antwortet Huma automatisch mit HTTP 422 und
// einem strukturierten JSON-Body — kein manuelles BindAndValidate nötig.
//
// BindAndValidate und CustomValidator werden nur noch für die wenigen
// verbliebenen Raw-Echo-Endpoints (SSE, WebSocket) benötigt.

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v5"
)

type CustomValidator struct {
	v *validator.Validate
}

func NewValidator() *CustomValidator {
	v := validator.New()
	v.RegisterTagNameFunc(
		func(fld reflect.StructField) string {
			name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
			if name == "-" {
				return ""
			}
			return name
		},
	)
	return &CustomValidator{v: v}
}

func (cv *CustomValidator) Validate(i interface{}) error {
	if err := cv.v.Struct(i); err != nil {
		return buildValidationError(err)
	}
	return nil
}

// BindAndValidate wird nur noch für Raw-Echo-Endpoints gebraucht.
// Für Huma-Handler: Input-Struct-Tags verwenden, Huma validiert automatisch.
func BindAndValidate(c *echo.Context, req interface{}) error {
	if err := c.Bind(req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "invalid request body"})
	}
	if err := c.Validate(req); err != nil {
		return c.JSON(http.StatusBadRequest, err)
	}
	return nil
}

type ValidationErrorResponse struct {
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

func (e *ValidationErrorResponse) Error() string { return e.Message }

func buildValidationError(err error) error {
	var ve validator.ValidationErrors
	if !errors.As(err, &ve) {
		return &ValidationErrorResponse{Message: err.Error()}
	}
	fields := make(map[string]string, len(ve))
	for _, fe := range ve {
		fields[fe.Field()] = fieldErrorMessage(fe)
	}
	return &ValidationErrorResponse{Message: "validation failed", Fields: fields}
}

func fieldErrorMessage(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "is required"
	case "email":
		return "must be a valid email address"
	case "min":
		return fmt.Sprintf("must be at least %s characters", fe.Param())
	case "max":
		return fmt.Sprintf("must be at most %s characters", fe.Param())
	case "oneof":
		return fmt.Sprintf("must be one of: %s", strings.ReplaceAll(fe.Param(), " ", ", "))
	case "url":
		return "must be a valid URL"
	case "gte":
		return fmt.Sprintf("must be >= %s", fe.Param())
	case "lte":
		return fmt.Sprintf("must be <= %s", fe.Param())
	case "gt":
		return fmt.Sprintf("must be > %s", fe.Param())
	case "lt":
		return fmt.Sprintf("must be < %s", fe.Param())
	case "len":
		return fmt.Sprintf("must be exactly %s characters", fe.Param())
	case "alphanum":
		return "must contain only letters and numbers"
	case "uuid":
		return "must be a valid UUID"
	default:
		return fmt.Sprintf("failed validation: %s", fe.Tag())
	}
}
