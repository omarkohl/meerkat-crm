package middleware

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/go-playground/validator/v10"
)

// validateJSON names fields by their JSON name, as the huma operations report them.
var validateJSON *validator.Validate

func init() {
	validateJSON = newValidator()
	validateJSON.RegisterTagNameFunc(jsonFieldName)
}

// jsonFieldName names a struct field by its JSON tag (falling back to the Go name).
func jsonFieldName(f reflect.StructField) string {
	name := strings.Split(f.Tag.Get("json"), ",")[0]
	if name == "-" {
		return ""
	}
	if name == "" {
		return f.Name
	}
	return name
}

func newValidator() *validator.Validate {
	validate := validator.New()

	// Register custom validators
	validators := []struct {
		tag string
		fn  validator.Func
	}{
		{"phone", validatePhone},
		{"birthday", validateBirthday},
		{"deceased_date", validateDeceasedDate},
		{"strong_password", validateStrongPassword},
		{"unique_circles", validateUniqueCircles},
		{"no_at_sign", validateNoAtSign},
		{"safeurl", validateSafeURL},
	}
	for _, v := range validators {
		if err := validate.RegisterValidation(v.tag, v.fn); err != nil {
			panic(fmt.Sprintf("register validator %q: %v", v.tag, err))
		}
	}
	return validate
}

// ValidationError represents a validation error response
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidateStructJSON validates a struct and returns formatted errors named by JSON field:
// "email" and "emails[0].value".
func ValidateStructJSON(obj interface{}) []ValidationError {
	var errors []ValidationError
	if err := validateJSON.Struct(obj); err != nil {
		for _, fe := range err.(validator.ValidationErrors) {
			// Namespace is "RootType.emails[0].value": drop the root type name.
			path := fe.Namespace()
			if i := strings.Index(path, "."); i >= 0 {
				path = path[i+1:]
			}
			errors = append(errors, ValidationError{Field: path, Message: formatMessage(fe, path)})
		}
	}
	return errors
}

func formatMessage(err validator.FieldError, field string) string {

	switch err.Tag() {
	case "required":
		return field + " is required"
	case "email":
		return field + " must be a valid email address"
	case "min":
		return field + " must be at least " + err.Param() + " characters"
	case "max":
		return field + " must be at most " + err.Param() + " characters"
	case "phone":
		return field + " must be a valid phone number"
	case "birthday":
		return field + " must be in YYYY-MM-DD format (use --MM-DD if year unknown)"
	case "deceased_date":
		return field + " must be a valid past date (YYYY-MM-DD) on or after the birthday"
	case "strong_password":
		return field + " is too weak. Use a longer password (15+ characters) or a passphrase (20+ characters). Avoid common passwords."
	case "unique_circles":
		return field + " cannot contain duplicate circles"
	case "no_at_sign":
		return field + " cannot contain the @ character"
	case "url":
		return field + " must be a valid URL"
	case "safeurl":
		return field + " uses an unsafe URL scheme"
	default:
		return field + " is invalid"
	}
}

// validatePhone validates phone number format
// Accepts: +1234567890, (123) 456-7890, 123-456-7890, etc.
func validatePhone(fl validator.FieldLevel) bool {
	phone := fl.Field().String()
	if phone == "" {
		return true // Allow empty (use 'required' tag if needed)
	}

	// Remove common formatting characters
	cleaned := strings.Map(func(r rune) rune {
		if unicode.IsDigit(r) || r == '+' {
			return r
		}
		return -1
	}, phone)

	// Must have between 5 and 20 digits
	if len(cleaned) < 5 || len(cleaned) > 20 {
		return false
	}

	return true
}

// rejects values whose URL scheme can execute scripts when the value is rendered as link
func validateSafeURL(fl validator.FieldLevel) bool {
	raw := strings.TrimSpace(fl.Field().String())
	if raw == "" {
		return true
	}
	normalized := strings.Map(func(r rune) rune {
		if r <= ' ' {
			return -1
		}
		return unicode.ToLower(r)
	}, raw)
	// Only a leading "scheme:" is dangerous; a bare "host:port" or path is fine.
	if i := strings.IndexByte(normalized, ':'); i > 0 {
		switch normalized[:i] {
		case "javascript", "data", "vbscript", "file":
			return false
		}
	}
	return true
}

// validateBirthday validates date format (YYYY-MM-DD or --MM-DD)
func validateBirthday(fl validator.FieldLevel) bool {
	birthday := fl.Field().String()
	if birthday == "" {
		return true // Allow empty (use 'required' tag if needed)
	}

	// Check format YYYY-MM-DD or --MM-DD (ISO 8601 format, year optional)
	match, _ := regexp.MatchString(`^(--|\d{4}-)\d{2}-\d{2}$`, birthday)
	if !match {
		return false
	}

	// Additional validation could check if date is valid
	return true
}

// validateDeceasedDate checks a full YYYY-MM-DD date that is not before a known birthday.
// "Not in the future" is the use case's check, which has the clock.
func validateDeceasedDate(fl validator.FieldLevel) bool {
	deceased := fl.Field().String()
	if deceased == "" {
		return true
	}
	parsed, err := time.Parse("2006-01-02", deceased)
	if err != nil {
		return false
	}
	birthday := fl.Parent().FieldByName("Birthday")
	if birthday.IsValid() && birthday.Kind() == reflect.String && !strings.HasPrefix(birthday.String(), "--") {
		if b, err := time.Parse("2006-01-02", birthday.String()); err == nil && parsed.Before(b) {
			return false
		}
	}
	return true
}

// validateStrongPassword checks password strength based on entropy
func validateStrongPassword(fl validator.FieldLevel) bool {
	password := fl.Field().String()

	// Check if it's a common password
	if IsCommonPassword(password) {
		return false
	}

	// Check entropy
	err := ValidatePasswordStrength(password)
	return err == nil
}

// validateUniqueCircles checks that all circles in the slice are unique (case-insensitive)
func validateUniqueCircles(fl validator.FieldLevel) bool {
	circles := fl.Field()
	if circles.Kind() != reflect.Slice {
		return true // Not a slice, let other validators handle this
	}

	seen := make(map[string]bool)
	for i := 0; i < circles.Len(); i++ {
		circle := circles.Index(i)
		if circle.Kind() != reflect.String {
			continue // Skip non-string elements
		}

		circleValue := strings.ToLower(strings.TrimSpace(circle.String()))
		if circleValue == "" {
			continue // Skip empty strings
		}

		if seen[circleValue] {
			return false // Duplicate found
		}
		seen[circleValue] = true
	}

	return true
}

// validateNoAtSign checks that a string field does not contain the @ character
func validateNoAtSign(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	return !strings.Contains(value, "@")
}

// ValidateEmail validates email format with regex
func ValidateEmail(email string) bool {
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	return emailRegex.MatchString(email)
}
