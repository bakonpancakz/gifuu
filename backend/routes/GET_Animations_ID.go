package routes

import (
	"gifuu/tools"
	"net/http"

	"github.com/jackc/pgx/v5"
)

func GET_Animations_ID(w http.ResponseWriter, r *http.Request) {

	ok, givenID := tools.GetPathID(w, r)
	if !ok {
		return
	}

	var Output []byte
	err := tools.Database.QueryRow(r.Context(),
		`SELECT jsonb_build_object(
			'id',     	u.id::text,
			'created',	u.created::timestamptz,
			'width',  	u.width,
			'height', 	u.height,
			'rating',	u.rating,
			'sticker',	u.sticker,
			'title',  	u.title,
			'tags',   	COALESCE(
				jsonb_agg(
					CASE WHEN t.id IS NOT NULL THEN
						jsonb_build_object(
							'id',    t.id::text,
							'label', t.label,
							'usage', t.usage
						)
					END
				) FILTER (WHERE t.id IS NOT NULL),
				'[]'
			)
		)
		FROM gifuu.upload u
		LEFT JOIN gifuu.upload_tag ut ON ut.gif_id = u.id
		LEFT JOIN gifuu.tag t ON t.id = ut.tag_id
		WHERE u.id = $1 AND u.rating < $2
		GROUP BY u.id`,
		givenID,
		tools.MODEL_THRESHOLD_HIDE,
	).Scan(&Output)

	if err == pgx.ErrNoRows {
		tools.SendClientError(w, r, tools.ERROR_UNKNOWN_ANIMATION)
		return
	}
	if err != nil {
		tools.SendServerError(w, r, err)
		return
	}

	tools.SendJSON(w, r, http.StatusOK, Output)
}
