package routes

import (
	"gifuu/tools"
	"net/http"
)

func POST_Tags_Autocomplete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var Body struct {
		Limit int    `json:"limit" validate:"required,min=1,max=100"`
		Query string `json:"query" validate:"required,tag"`
	}
	if !tools.BindJSON(w, r, &Body) {
		return
	}

	var Object []byte
	err := tools.Database.QueryRow(ctx,
		`SELECT COALESCE(jsonb_agg(t), '[]') FROM (
			SELECT id::text AS id, label, usage
			FROM gifuu.tag
			ORDER BY similarity(label, $1) DESC, usage DESC
			LIMIT $2
		) t`,
		("%" + Body.Query + "%"),
		Body.Limit,
	).Scan(&Object)

	if err != nil {
		tools.SendServerError(w, r, err)
		return
	}

	tools.SendJSON(w, r, http.StatusOK, Object)
}
