package routes

import (
	"gifuu/tools"
	"net/http"
)

func POST_Animations_Search(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var Body struct {
		Limit   int               `json:"limit" validate:"required,min=1,max=100"`
		AfterID tools.Snowflake   `json:"after_id" validate:"omitempty,min=1"`
		Tags    []tools.Snowflake `json:"query" validate:"required,unique,min=1,max=32"`
	}
	if !tools.BindJSON(w, r, &Body) {
		return
	}

	var Object []byte
	err := tools.Database.QueryRow(ctx,
		`SELECT COALESCE(jsonb_agg(t.obj), '[]') FROM (
			SELECT jsonb_build_object(
				'id',      u.id::text,
				'created', u.created::timestamptz,
				'sticker', u.sticker,
				'width',   u.width,
				'height',  u.height,
				'rating',  u.rating,
				'title',   u.title,
				'tags',    COALESCE(
					jsonb_agg(
						jsonb_build_object(
							'id',    t.id::text,
							'label', t.label,
							'usage', t.usage
						)
						ORDER BY t.usage DESC
					) FILTER (WHERE t.id IS NOT NULL),
					'[]'
				)
			) AS obj
			FROM gifuu.upload u
			LEFT JOIN gifuu.upload_tag ut ON ut.gif_id = u.id
			LEFT JOIN gifuu.tag t ON t.id = ut.tag_id
			WHERE ut.tag_id = ANY($1::bigint[])
			AND ($2::bigint = 0 OR u.id < $2::bigint)
			AND u.rating < $4
			GROUP BY u.id
			HAVING COUNT(DISTINCT ut.tag_id) = cardinality($1::bigint[])
			ORDER BY u.id DESC
			LIMIT $3::int
		) t`,
		Body.Tags,
		Body.AfterID,
		Body.Limit,
		tools.MODEL_THRESHOLD_HIDE,
	).Scan(&Object)
	if err != nil {
		tools.SendServerError(w, r, err)
		return
	}

	tools.SendJSON(w, r, http.StatusOK, Object)
}
