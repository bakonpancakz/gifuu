package tools

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/bits"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

type RatelimitOptions struct {
	Bucket string        // Bucket Name
	Limit  int           // Maximum Amount of Requests
	Period time.Duration // Reset Period
}

// Protect Server against Abuse by Limiting the amount of incoming requests
func NewRatelimit(bucket string, limit int, period time.Duration) MiddlewareFunc {
	return func(w http.ResponseWriter, r *http.Request) bool {

		// Check Ratelimit Token
		if t := r.Header.Get("X-Ratelimit-Token"); t != "" {
			var BypassID int64
			err := Database.QueryRow(r.Context(),
				"SELECT id FROM gifuu.ratelimit_bypass WHERE token = $1",
				GenerateHash(t),
			).Scan(
				&BypassID,
			)
			if err == nil {
				w.Header().Add("X-Ratelimit-Token-Id", strconv.FormatInt(BypassID, 10))
				return true
			}
		}

		// Calculate Ratelimit
		var bucketSubject = (bucket + ":" + GetRemoteIPHash(r))
		var bucketCreated time.Time
		var bucketUsage int

		err := Database.QueryRow(r.Context(),
			`INSERT INTO gifuu.ratelimit_usage (subject, created, usage)
			VALUES ($1, NOW(), 1)
			ON CONFLICT (subject) DO UPDATE
			SET usage = CASE
				WHEN gifuu.ratelimit_usage.created + ($2 * interval '1 microsecond') > NOW()
				THEN gifuu.ratelimit_usage.usage + 1
				ELSE 1
			END,
			created = CASE
				WHEN gifuu.ratelimit_usage.created + ($2 * interval '1 microsecond') > NOW()
				THEN gifuu.ratelimit_usage.created
				ELSE NOW()
			END
			RETURNING created, usage`,
			bucketSubject,
			period.Microseconds(),
		).Scan(
			&bucketCreated,
			&bucketUsage,
		)
		if err != nil {
			SendServerError(w, r, err)
			return false
		}

		// Enforce Ratelimit
		var (
			valueLimit  = strconv.Itoa(limit)
			valueRemain = strconv.Itoa(max(limit-bucketUsage, 0))
			valueReset  = strconv.FormatFloat(time.Until(bucketCreated.Add(period)).Seconds(), 'f', 2, 64)
		)
		w.Header().Set("X-Ratelimit-Bucket", bucket)
		w.Header().Set("X-Ratelimit-Limit", valueLimit)
		w.Header().Set("X-Ratelimit-Reset", valueReset)
		w.Header().Set("X-Ratelimit-Remaining", valueRemain)

		if bucketUsage > limit {
			SendClientError(w, r, ERROR_GENERIC_RATELIMIT)
			return false
		}

		return true
	}
}

// Protect Server against Abuse by requiring a valid Proof of Work Token
func RequirePoW(w http.ResponseWriter, r *http.Request) bool {
	var givenNonce = r.Header.Get("X-PoW-Nonce")
	var givenCounter = r.Header.Get("X-PoW-Counter")
	var givenCounterInt = 0

	// Parse Headers
	if _, err := hex.DecodeString(givenNonce); err != nil {
		SendClientError(w, r, ERROR_POW_INVALID)
		return false
	}
	if v, err := strconv.Atoi(givenCounter); err != nil || v < 0 {
		SendClientError(w, r, ERROR_POW_INVALID)
		return false
	} else {
		givenCounterInt = v
	}

	// Consume Session
	var difficulty int
	err := Database.QueryRow(r.Context(),
		`DELETE FROM gifuu.session_pow
		WHERE nonce = $1 AND expires > NOW()
		RETURNING difficulty`,
		givenNonce,
	).Scan(
		&difficulty,
	)

	if err == pgx.ErrNoRows {
		SendClientError(w, r, ERROR_POW_INVALID)
		return false
	}
	if err != nil {
		SendServerError(w, r, err)
		return false
	}

	// Validate Session
	input := fmt.Sprintf("%s%d", givenNonce, givenCounterInt)
	hash := sha256.Sum256([]byte(input))

	zeroBitsRequired := difficulty
	zeroBitsFound := 0
	for _, b := range hash {
		if b == 0 {
			zeroBitsFound += 8
		} else {
			zeroBitsFound += bits.LeadingZeros8(b)
			break
		}
	}
	if zeroBitsFound < zeroBitsRequired {
		SendClientError(w, r, ERROR_POW_INVALID)
		return false
	}

	return true
}

func RequireCORS(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Pow-Nonce, X-Pow-Counter")
		w.WriteHeader(http.StatusNoContent)
		return false
	}
	return true
}

type MiddlewareFunc func(w http.ResponseWriter, r *http.Request) bool

// Apply Middleware before Processing Request
func Chain(h http.HandlerFunc, mw ...MiddlewareFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		for i := 0; i < len(mw); i++ {
			if !mw[i](w, r) {
				return
			}
		}
		h(w, r)
	}
}

type MethodHandler map[string]http.HandlerFunc

func (mh MethodHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !RequireCORS(w, r) {
		return
	}
	if handler, ok := mh[r.Method]; ok {
		handler(w, r)
	} else {
		SendClientError(w, r, ERROR_GENERIC_METHOD_NOT_ALLOWED)
	}
}
