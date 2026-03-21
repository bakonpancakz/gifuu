package tools

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
)

var (
	BodyValidator = validator.New(validator.WithRequiredStructEnabled())
	RegexTag      = regexp.MustCompile(`^[\p{L}\p{N}_]+$`)
)

func init() {

	BodyValidator.RegisterValidation("tag", func(fl validator.FieldLevel) bool {
		str := fl.Field().String()
		trm := strings.TrimSpace(str)
		if len(str) != len(trm) || len(str) > 64 ||
			strings.Contains(str, "  ") ||
			!RegexTag.MatchString(str) {
			return false
		}
		return true
	})

}

// Decode Incoming JSON Request
func BindJSON(w http.ResponseWriter, r *http.Request, b any) bool {

	// Header Validation
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		SendClientError(w, r, ERROR_BODY_INVALID_CONTENT_TYPE)
		return false
	}
	if r.ContentLength > int64(LIMIT_JSON) {
		SendClientError(w, r, ERROR_BODY_TOO_LARGE)
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, int64(LIMIT_JSON))
	defer r.Body.Close()

	// Decode into Struct
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(b); err != nil {
		SendClientError(w, r, ERROR_BODY_INVALID_DATA)
		return false
	}

	// Struct Validation
	if err := BodyValidator.Struct(b); err != nil {
		SendClientError(w, r, ERROR_BODY_INVALID_FIELD)
		return false
	}

	return true
}

// Get Snowflake from Request Path.
// Expects id to be present in http handler (e.g. '/path/to/item/{id}')
func GetPathID(w http.ResponseWriter, r *http.Request) (bool, int64) {
	v, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || v < 1 {
		SendClientError(w, r, ERROR_BODY_INVALID_FIELD)
		return false, 0
	}
	return true, v
}

// Get IP Address of Incoming Client as Hex-Encoded SHA256 String for Privacy
func GetRemoteIPHash(r *http.Request) string {
	return fmt.Sprintf("%x", sha256.Sum256(
		[]byte(GetRemoteIP(r)),
	))
}

// Get IP Address of Incoming Client
func GetRemoteIP(r *http.Request) string {
	remoteAddr, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return ""
	}
	clientIP := net.ParseIP(remoteAddr)

	// Skip if request was not proxied
	if len(HTTP_IP_HEADERS) == 0 || len(HTTP_IP_PROXIES) == 0 {
		return clientIP.String()
	}

	// Walk through headers in configured order (most recent first)
	// Scan Headers as Configured by Environment
	for _, header := range HTTP_IP_HEADERS {
		hv := r.Header.Get(header)
		if hv == "" {
			continue
		}
		for _, ipStr := range strings.Split(hv, ",") {
			ipStr = strings.TrimSpace(ipStr)
			ip := net.ParseIP(ipStr)
			if ip != nil && !isTrustedProxy(ip) {
				return ip.String()
			}
		}
	}

	// Proxy is misconfigured, use fallback!
	return clientIP.String()
}

func isTrustedProxy(ip net.IP) bool {
	for _, cidr := range HTTP_IP_PROXIES {
		if _, network, err := net.ParseCIDR(cidr); err == nil {
			if network.Contains(ip) {
				return true
			}
		} else if proxyIP := net.ParseIP(cidr); proxyIP != nil && proxyIP.Equal(ip) {
			return true
		}
	}
	return false
}
