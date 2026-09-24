package docuware

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ErrNoCredentials means a request needs a token and none can be obtained.
var ErrNoCredentials = errors.New("not logged in to DocuWare")

// ErrNoText means a document section has no OCR fulltext (textshot).
var ErrNoText = errors.New("no fulltext available: the file cabinet may not be fulltext-indexed, or the document is not processed yet")

// APIError is a non-2xx response from DocuWare.
type APIError struct {
	Method  string
	URL     string
	Status  int
	Message string
}

func (e *APIError) Error() string {
	msg := fmt.Sprintf("%s %s: HTTP %d", e.Method, e.URL, e.Status)
	if e.Message != "" {
		msg += ": " + e.Message
	}
	return msg
}

// AuthError reports a login rejected by the Identity Service.
type AuthError struct{ Reason string }

func (e *AuthError) Error() string { return "DocuWare rejected the login: " + e.Reason }

// NotFoundError reports an unknown file cabinet, dialog or field, listing the valid choices.
type NotFoundError struct {
	Kind      string
	Key       string
	Available []string
}

func (e *NotFoundError) Error() string {
	msg := fmt.Sprintf("%s %q not found", e.Kind, e.Key)
	if len(e.Available) > 0 {
		msg += "; available: " + strings.Join(e.Available, ", ")
	}
	return msg
}

func newAPIError(resp *http.Response) *APIError {
	e := &APIError{Status: resp.StatusCode}
	if resp.Request != nil {
		e.Method = resp.Request.Method
		e.URL = resp.Request.URL.RequestURI()
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	var payload struct {
		Message     string `json:"Message"`
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	if json.Unmarshal(body, &payload) == nil {
		e.Message = firstNonEmpty(payload.Message, payload.Description, payload.Error)
	}
	if e.Message == "" {
		text := strings.TrimSpace(string(body))
		if len(text) > 0 && len(text) <= 300 && !strings.HasPrefix(text, "<") {
			e.Message = text
		}
	}
	return e
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
