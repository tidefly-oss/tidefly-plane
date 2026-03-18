package middleware

// maxBodySize ist das globale Limit für Request/Response-Body-Logging (2 KB).
const maxBodySize = 64 * 1024

// sensitiveFields werden aus JSON-Bodies vor dem Logging redigiert.
var sensitiveFields = map[string]bool{
	"password":         true,
	"password_confirm": true,
	"token":            true,
	"access_token":     true,
	"refresh_token":    true,
	"secret":           true,
	"secret_key":       true,
	"api_key":          true,
	"private_key":      true,
	"authorization":    true,
	"csrf_token":       true,
	"session_token":    true,
}

// loggableContentTypes — nur diese Bodies werden gelesen/geloggt.
var loggableContentTypes = []string{
	"application/json",
	"application/x-www-form-urlencoded",
	"text/plain",
}
