package tools

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime/debug"
	"strings"
)

type APIError struct {
	Status  int    `json:"-"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

var (
	ERROR_GENERIC_SERVER             = APIError{Status: 500, Code: 0, Message: "Server Error"}
	ERROR_GENERIC_NOT_FOUND          = APIError{Status: 404, Code: 0, Message: "Endpoint Not Found"}
	ERROR_GENERIC_RATELIMIT          = APIError{Status: 429, Code: 0, Message: "Too Many Requests"}
	ERROR_GENERIC_UNAUTHORIZED       = APIError{Status: 401, Code: 0, Message: "Unauthorized"}
	ERROR_GENERIC_FORBIDDEN          = APIError{Status: 403, Code: 0, Message: "Forbidden"}
	ERROR_GENERIC_METHOD_NOT_ALLOWED = APIError{Status: 405, Code: 0, Message: "Method Not Allowed"}
	ERROR_SERVER_RESOURCES_EXHAUSTED = APIError{Status: 507, Code: 0, Message: "Server Resources Exhausted"}
	ERROR_BODY_EMPTY                 = APIError{Status: 411, Code: 0, Message: "Request Body is Empty"}
	ERROR_BODY_TOO_LARGE             = APIError{Status: 413, Code: 0, Message: "Request Body is Too Large"}
	ERROR_BODY_INVALID_DATA          = APIError{Status: 422, Code: 0, Message: "Invalid Body"}
	ERROR_BODY_INVALID_CONTENT_TYPE  = APIError{Status: 400, Code: 0, Message: "Invalid 'Content-Type' Header"}
	ERROR_BODY_INVALID_FIELD         = APIError{Status: 400, Code: 0, Message: "Invalid Body Field"}
	ERROR_UNKNOWN_ENDPOINT           = APIError{Status: 404, Code: 0, Message: "Unknown Endpoint"}
	ERROR_UNKNOWN_ANIMATION          = APIError{Status: 404, Code: 0, Message: "Unknown Animation"}
	ERROR_POW_INVALID                = APIError{Status: 400, Code: 0, Message: "Invalid or Expired PoW"}
	ERROR_MEDIA_INVALID              = APIError{Status: 400, Code: 0, Message: "Media Invalid"}
	ERROR_MEDIA_INAPPROPRIATE        = APIError{Status: 400, Code: 0, Message: "Media Inappropriate"}
)

// Cancel the request with an API Error (Slightly more optimized than SendJSON)
func SendClientError(w http.ResponseWriter, r *http.Request, e APIError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.Status)
	fmt.Fprintf(w, `{"code":%d,"message":%q}`, e.Code, e.Message)
}

// Cancel the request with a generic server error.
// Additionally collects debug about the error and sends it to the logger.
func SendServerError(w http.ResponseWriter, r *http.Request, err error) {

	debugStack := strings.Split(string(debug.Stack()), "\n")
	for i, item := range debugStack {
		debugStack[i] = strings.ReplaceAll(item, "\t", "    ")
	}
	if len(debugStack) > 5 {
		debugStack = debugStack[5:] // skip header
	}

	reqHeader := make(map[string]string, len(r.Header))
	for key, header := range r.Header {
		reqHeader[key] = strings.Join(header, ", ")
	}

	LoggerHTTP.Data(ERROR, err.Error(), map[string]any{
		"request": map[string]any{
			"method":  r.Method,
			"url":     r.URL.String(),
			"headers": reqHeader,
		},
		"error": map[string]any{
			"raw":     err,
			"message": err.Error(),
			"stack":   debugStack,
		},
	})

	if w != nil {
		SendClientError(w, r, ERROR_GENERIC_SERVER)
	}
}

// Respond to the request with a JSON object, additionally uses GZIP if the client supports it.
func SendJSON(w http.ResponseWriter, r *http.Request, statusCode int, responseObject any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	// Setup Compression
	var wr io.Writer = w
	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		defer gz.Close()
		wr = gz
	}
	w.WriteHeader(statusCode)

	// Encode Content
	if b, ok := responseObject.([]byte); ok {
		_, err := wr.Write(b)
		return err
	}
	enc := json.NewEncoder(wr)
	enc.SetEscapeHTML(false)
	return enc.Encode(responseObject)
}
