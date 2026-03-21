package routes

import (
	"context"
	"gifuu/tools"
	"net/http"
	"path"
	"strconv"
)

func DELETE_Animations_ID(w http.ResponseWriter, r *http.Request) {
	ok, givenID := tools.GetPathID(w, r)
	if !ok {
		return
	}

	// Delete Upload
	tag, err := tools.Database.Exec(r.Context(),
		"DELETE FROM gifuu.upload WHERE id = $1 AND upload_token = $2",
		givenID,
		tools.GenerateHash(r.Header.Get("Authorization")),
	)
	if err != nil {
		tools.SendServerError(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		tools.SendClientError(w, r, tools.ERROR_GENERIC_UNAUTHORIZED)
		return
	}

	// Delete Images
	ctx, cancel := context.WithTimeout(context.Background(), tools.TIMEOUT_CONTEXT)
	defer cancel()

	idString := strconv.FormatInt(givenID, 10)
	if err := tools.Storage.Delete(ctx,
		path.Join(idString, MEDIA_FILENAME_PREVIEW),
		path.Join(idString, MEDIA_FILENAME_STANDARD),
		path.Join(idString, MEDIA_FILENAME_ALPHA),
	); err != nil {
		tools.LoggerStorage.Log(tools.WARN,
			"Failed to delete images for upload (%s): %s", idString, err)
	}

	w.WriteHeader(http.StatusNoContent)
}
