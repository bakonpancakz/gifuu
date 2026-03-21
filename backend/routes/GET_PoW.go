package routes

import (
	"crypto/rand"
	"encoding/hex"
	"gifuu/tools"
	"net/http"
	"time"
)

func GET_PoW(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Generate Nonce
	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		tools.SendServerError(w, r, err)
		return
	}
	nonceStr := hex.EncodeToString(nonceBytes)
	nonceExp := time.Now().Add(tools.LIFETIME_POW_TOKEN)

	// Create Session
	if _, err := tools.Database.Exec(ctx,
		"INSERT INTO gifuu.session_pow (nonce, expires, difficulty) VALUES ($1, $2, $3)",
		nonceStr,
		nonceExp,
		tools.HTTP_DIFFICULTY,
	); err != nil {
		tools.SendServerError(w, r, err)
		return
	}

	// Organize Session
	tools.SendJSON(w, r, http.StatusOK, map[string]any{
		"nonce":      nonceStr,
		"difficulty": tools.HTTP_DIFFICULTY,
		"expires":    nonceExp.Unix(),
	})
}
