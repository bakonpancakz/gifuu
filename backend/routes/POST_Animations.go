package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"gifuu/tools"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	MEDIA_MAX_WIDTH         = 7680 // Allow 8K Image
	MEDIA_MAX_HEIGHT        = 4320
	MEDIA_MIN_WIDTH         = 32
	MEDIA_MIN_HEIGHT        = 32
	MEDIA_MAX_DURATION      = 35
	MEDIA_COLOR_SPACE       = "yuv420p"
	MEDIA_COLOR_RANGE       = "tv"
	MEDIA_FILENAME_PREVIEW  = "preview.avif"
	MEDIA_FILENAME_STANDARD = "standard.avif"
	MEDIA_FILENAME_ALPHA    = "alpha.webm"
	MEDIA_BACKGROUND        = "#ffffff"

	VIDEO_ENCODE_CODEC         = "libsvtav1"
	VIDEO_ENCODE_EFFORT        = "7"
	VIDEO_ENCODE_PARAMS        = "lp=1"
	VIDEO_FILTERS              = ""
	VIDEO_KEYFRAME_INTERVAL    = 4
	VIDEO_LARGE_MAX_FRAMES     = VIDEO_LARGE_FPS * MEDIA_MAX_DURATION
	VIDEO_LARGE_KEYINT         = VIDEO_LARGE_FPS * VIDEO_KEYFRAME_INTERVAL
	VIDEO_LARGE_FPS            = 48
	VIDEO_LARGE_SIZE           = 600
	VIDEO_LARGE_QUALITY        = "49"
	VIDEO_PREVIEW_MAX_DURATION = 6
	VIDEO_PREVIEW_MAX_FRAMES   = VIDEO_PREVIEW_FPS * VIDEO_PREVIEW_MAX_DURATION
	VIDEO_PREVIEW_KEYINT       = VIDEO_PREVIEW_FPS * VIDEO_KEYFRAME_INTERVAL
	VIDEO_PREVIEW_FPS          = 20
	VIDEO_PREVIEW_SIZE         = 200
	VIDEO_PREVIEW_QUALITY      = "53"

	IMAGE_ENCODE_CODEC   = "libsvtav1"
	IMAGE_ENCODE_EFFORT  = "7"
	IMAGE_ENCODE_QUALITY = "46"
	IMAGE_ENCODE_PARAMS  = "lp=1"
	IMAGE_FILTERS        = "tpad=stop_mode=clone:stop_duration=1,"
	IMAGE_LARGE_SIZE     = 2160
	IMAGE_PREVIEW_SIZE   = 240
)

func ternary[T any](cond bool, a, b T) T {
	if cond {
		return a
	}
	return b
}

func POST_Animations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// --------------------------------------------------------------------------------
	// --- [ Prevent Abuse ] ----------------------------------------------------------
	// --------------------------------------------------------------------------------
	var (
		ClientAddress = tools.GetRemoteIPHash(r)
		ClientToken   = tools.GenerateRandomToken()
	)
	{
		// --- Check Ban List ---
		var SubjectCount int
		err := tools.Database.QueryRow(ctx,
			"SELECT COUNT(*) FROM gifuu.mod_banned WHERE subject = $1",
			ClientAddress,
		).Scan(
			&SubjectCount,
		)
		if err != nil {
			tools.SendServerError(w, r, err)
			return
		}
		if SubjectCount > 0 {
			tools.SendClientError(w, r, tools.ERROR_GENERIC_FORBIDDEN)
			return
		}

		// --- Request Upload Capacity ---
		var (
			localLimit  = int64(tools.LIMIT_FILE)
			localUsage  = int64(r.ContentLength)
			globalLimit = int64(tools.LIMIT_TEMP)
			globalUsage int64
		)
		if localUsage > localLimit {
			tools.SendClientError(w, r, tools.ERROR_BODY_TOO_LARGE)
			return
		}
		for {
			globalUsage = tools.SYNC_UPLOADS.Load()
			if globalUsage+localUsage > globalLimit {
				tools.SendClientError(w, r, tools.ERROR_SERVER_RESOURCES_EXHAUSTED)
				return
			}
			if tools.SYNC_UPLOADS.CompareAndSwap(globalUsage, globalUsage+localUsage) {
				break // space reserved!
			}
		}
		defer tools.SYNC_UPLOADS.Add(-localUsage)
	}

	// --------------------------------------------------------------------------------
	// --- [ Parse Incoming Request ] -------------------------------------------------
	// --------------------------------------------------------------------------------
	var (
		TempID       = tools.GenerateSnowflake()     //
		TempIDString = strconv.FormatInt(TempID, 10) //
		TempSuccess  = false                         //
		TempUpload   *os.File                        // Request Media
		TempLogger   *os.File                        // Relevant Logs
		PathUpload   = filepath.Join(tools.STORAGE_DISK_TEMP, TempIDString+".bin")
		PathLogger   = filepath.Join(tools.STORAGE_DISK_TEMP, TempIDString+".log")
		PathAlpha    = filepath.Join(tools.STORAGE_DISK_TEMP, TempIDString+"_"+MEDIA_FILENAME_ALPHA)
		PathPreview  = filepath.Join(tools.STORAGE_DISK_TEMP, TempIDString+"_"+MEDIA_FILENAME_PREVIEW)
		PathStandard = filepath.Join(tools.STORAGE_DISK_TEMP, TempIDString+"_"+MEDIA_FILENAME_STANDARD)
		Body         struct {
			Title string   `json:"title" validate:"required,min=1,max=128"`
			Tags  []string `json:"tags" validate:"required,min=1,max=32,dive,tag"`
		}
	)
	{
		w.Header().Set("X-Debug-ID", TempIDString)

		// --- Create Temporary Files ---
		ff := os.O_CREATE | os.O_TRUNC | os.O_WRONLY
		fp := tools.FILE_MODE

		if f, err := os.OpenFile(PathUpload, ff, fp); err != nil {
			tools.SendServerError(w, r, err)
			return
		} else {
			TempUpload = f
			defer os.Remove(PathUpload)
			defer f.Close()
		}

		if f, err := os.OpenFile(PathLogger, ff, fp); err != nil {
			tools.SendServerError(w, r, err)
			return
		} else {
			TempLogger = f
			defer f.Close()
		}

		defer os.Remove(PathAlpha)
		defer os.Remove(PathPreview)
		defer os.Remove(PathStandard)

		// --- Parse Form Body ---
		var haveFile, haveData bool
		reader, err := r.MultipartReader()
		if err != nil {
			tools.SendClientError(w, r, tools.ERROR_BODY_INVALID_DATA)
			return
		}
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}

			// --- Parse Incoming Metadata ---
			if part.FormName() == "data" {
				if haveData {
					tools.SendClientError(w, r, tools.ERROR_BODY_INVALID_FIELD)
					return
				}
				haveData = true

				raw, err := io.ReadAll(io.LimitReader(part, int64(tools.LIMIT_JSON)))
				if err != nil {
					tools.SendClientError(w, r, tools.ERROR_BODY_INVALID_FIELD)
					break
				}
				if err := json.Unmarshal(raw, &Body); err != nil {
					tools.SendClientError(w, r, tools.ERROR_BODY_INVALID_DATA)
					return
				}
				if err := tools.BodyValidator.Struct(Body); err != nil {
					tools.SendClientError(w, r, tools.ERROR_BODY_INVALID_FIELD)
					return
				}

				fmt.Fprintf(TempLogger, "Collected JSON : %s\n", raw)
				continue
			}

			// --- Store Incoming Upload ---
			if part.FormName() == "file" {

				if haveFile {
					tools.SendClientError(w, r, tools.ERROR_BODY_INVALID_FIELD)
					return
				}
				haveFile = true

				mediaType := part.Header.Get("Content-Type")
				if true &&
					!strings.HasPrefix(mediaType, "image/") &&
					!strings.HasPrefix(mediaType, "video/") {
					tools.SendClientError(w, r, tools.ERROR_BODY_INVALID_CONTENT_TYPE)
					return
				}
				mediaSize, err := io.Copy(TempUpload, io.LimitReader(part, int64(tools.LIMIT_FILE)))
				if err != nil {
					tools.SendClientError(w, r, tools.ERROR_BODY_INVALID_DATA)
					return
				}
				TempUpload.Close()

				fmt.Fprintf(TempLogger,
					"Collected File : %s (Type: %s) (Size: %db)\n",
					mediaType, part.FileName(), mediaSize,
				)
				continue
			}

			tools.SendClientError(w, r, tools.ERROR_BODY_INVALID_FIELD)
			return
		}

		if !haveFile || !haveData {
			tools.SendClientError(w, r, tools.ERROR_BODY_INVALID_FIELD)
			return
		}
	}

	// --------------------------------------------------------------------------------
	// --- [ Media Validation ] -------------------------------------------------------
	// --------------------------------------------------------------------------------
	var (
		ProbeResults tools.ProbeResults // Probe Results
		ProbeStream  *tools.ProbeStream // Relevant Media Stream
		MediaSticker bool               // Is Sticker?
		MediaHeight  int                // Approximate Scaled Height
		MediaWidth   int                // Approximate Scaled Width
		MediaRating  float32            // Worst value from Classification
	)
	{
		t := time.Now()

		// --- Probe Media Stream ---
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		cmd := exec.CommandContext(ctx, "ffprobe",
			"-hide_banner",
			"-loglevel", "verbose",
			"-print_format", "json",
			"-show_streams",
			"-i", PathUpload,
		)
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()

		fmt.Fprintf(TempLogger, "\n%s\n%s\nProbing completed in %s\n\n",
			// extra newline for proper log padding
			stdout.String(),
			stderr.String(),
			time.Since(t),
		)

		if err != nil {
			tools.SendServerError(w, r, err)
			return
		}
		if err := json.Unmarshal(stdout.Bytes(), &ProbeResults); err != nil {
			tools.SendServerError(w, r, err)
			return
		}

		// --- Find Suitable Media Stream ---
		//	We only accept images if it's the only available stream otherwise
		// 	video files with an embedded thumbnail will become an image
		for _, s := range ProbeResults.Streams {
			if s.CodecType == "video" || (s.CodecType == "image" && len(ProbeResults.Streams) == 1) {
				if false ||
					s.Height < MEDIA_MIN_HEIGHT || s.Height > MEDIA_MAX_HEIGHT ||
					s.Width < MEDIA_MIN_WIDTH || s.Width > MEDIA_MAX_WIDTH ||
					float64(s.Duration) > float64(MEDIA_MAX_DURATION) {
					continue
				}

				ProbeStream = &s
				break
			}
		}
		if ProbeStream == nil {
			tools.SendClientError(w, r, tools.ERROR_MEDIA_INVALID)
			return
		}

		// --- Calculate Pipeline ---
		// 	Metadata fields and sticker detection
		MediaSticker = (ProbeStream.NumberFrames < 2)
		d := ternary(MediaSticker, IMAGE_LARGE_SIZE, VIDEO_LARGE_SIZE)
		s := float64(min(ProbeStream.Height, d)) / float64(ProbeStream.Height)
		MediaHeight = int(float64(ProbeStream.Height)*s) &^ 1
		MediaWidth = int(float64(ProbeStream.Width)*s) &^ 1
	}

	// --------------------------------------------------------------------------------
	// --- [ Media Processing ] -------------------------------------------------------
	// --------------------------------------------------------------------------------
	{
		t := time.Now()

		// --- Acquire Upload Slot ---
		if err := tools.SYNC_ENCODES.Acquire(ctx); err != nil {
			tools.SendClientError(w, r, tools.ERROR_SERVER_RESOURCES_EXHAUSTED)
			return
		}
		defer tools.SYNC_ENCODES.Release()

		// --- Prepare For Encoding ---
		var (
			encodeCtx, cancel = context.WithCancel(ctx)
			stderr            bytes.Buffer
		)
		defer cancel()

		cmd := exec.CommandContext(encodeCtx,
			"ffmpeg",
			"-hide_banner",
			"-loglevel", "verbose",
			"-stats", "-an", "-sn", "-y",
			"-i", PathUpload,

			"-filter_complex", fmt.Sprintf(""+
				"[0:v]format=rgba[fg];[fg]split[fg1][fg2];[fg2]drawbox=x=0:y=0:w=iw:h=ih:color=%s:t=fill[bg];[bg][fg1]overlay[base];"+
				"[base]split=4[v1][v2][v3][v4];"+
				"[v1]%sscale=-2:%d:flags=lanczos,fps=%d[v1o];"+
				"[v2]%sscale=-2:%d:flags=lanczos,fps=%d[v2o];"+
				"[v3]%sscale=-2:%d:flags=lanczos,fps=%d[v3a];[v3a]format=rgba[v3f];[v3f]split[v3color][v3mask];[v3mask]alphaextract[v3ae];[v3color][v3ae]vstack[v3o];"+
				"[v4]%sscale=%d:%d:flags=neighbor,fps=%d[v4o];",

				// Import
				MEDIA_BACKGROUND,

				// Export: Preview
				ternary(MediaSticker, IMAGE_FILTERS, VIDEO_FILTERS),
				min(ProbeStream.Height, ternary(MediaSticker, IMAGE_PREVIEW_SIZE, VIDEO_PREVIEW_SIZE)),
				min(int(ProbeStream.RFrameRate), ternary(MediaSticker, 1, VIDEO_PREVIEW_FPS)),

				// Export: Standard
				ternary(MediaSticker, IMAGE_FILTERS, VIDEO_FILTERS),
				min(ProbeStream.Height, ternary(MediaSticker, IMAGE_LARGE_SIZE, VIDEO_LARGE_SIZE)),
				min(int(ProbeStream.RFrameRate), ternary(MediaSticker, 1, VIDEO_LARGE_FPS)),

				// Export: Alpha
				ternary(MediaSticker, IMAGE_FILTERS, VIDEO_FILTERS),
				min(ProbeStream.Height, ternary(MediaSticker, IMAGE_LARGE_SIZE, VIDEO_LARGE_SIZE)),
				min(int(ProbeStream.RFrameRate), ternary(MediaSticker, 1, VIDEO_LARGE_FPS)),

				// Export: Inference
				IMAGE_FILTERS,
				tools.MODEL_SIZE,
				tools.MODEL_SIZE,
				tools.MODEL_FRAMERATE,
			),

			// Export Preview
			"-map", "[v1o]",
			"-c:v" /*----------*/, ternary(MediaSticker, IMAGE_ENCODE_CODEC, VIDEO_ENCODE_CODEC),
			"-preset" /*-------*/, ternary(MediaSticker, IMAGE_ENCODE_EFFORT, VIDEO_ENCODE_EFFORT),
			"-qp" /*-----------*/, ternary(MediaSticker, IMAGE_ENCODE_QUALITY, VIDEO_PREVIEW_QUALITY),
			"-g" /*------------*/, strconv.Itoa(VIDEO_PREVIEW_KEYINT),
			"-pix_fmt" /*------*/, MEDIA_COLOR_SPACE,
			"-color_range" /*--*/, MEDIA_COLOR_RANGE,
			"-svtav1-params" /**/, ternary(MediaSticker, IMAGE_ENCODE_PARAMS, VIDEO_ENCODE_PARAMS),
			"-frames:v" /*-----*/, ternary(MediaSticker, "1", strconv.Itoa(VIDEO_PREVIEW_MAX_FRAMES)),
			"-loop" /*---------*/, ternary(MediaSticker, "1", "0"),
			"-map_metadata" /*-*/, "-1",
			"-metadata", ("gifuu_encoder=" + tools.MACHINE_HOSTNAME),
			"-metadata", ("gifuu_proverb=" + tools.MACHINE_PROVERB),
			PathPreview,

			// Export: Standard
			"-map", "[v2o]",
			"-c:v" /*----------*/, ternary(MediaSticker, IMAGE_ENCODE_CODEC, VIDEO_ENCODE_CODEC),
			"-preset" /*-------*/, ternary(MediaSticker, IMAGE_ENCODE_EFFORT, VIDEO_ENCODE_EFFORT),
			"-qp" /*-----------*/, ternary(MediaSticker, IMAGE_ENCODE_QUALITY, VIDEO_LARGE_QUALITY),
			"-g" /*------------*/, strconv.Itoa(VIDEO_LARGE_KEYINT),
			"-pix_fmt" /*------*/, MEDIA_COLOR_SPACE,
			"-color_range" /*--*/, MEDIA_COLOR_RANGE,
			"-svtav1-params" /**/, ternary(MediaSticker, IMAGE_ENCODE_PARAMS, VIDEO_ENCODE_PARAMS),
			"-frames:v" /*-----*/, ternary(MediaSticker, "1", strconv.Itoa(VIDEO_LARGE_MAX_FRAMES)),
			"-loop" /*---------*/, ternary(MediaSticker, "1", "0"),
			"-map_metadata" /*-*/, "-1",
			"-metadata", ("gifuu_encoder=" + tools.MACHINE_HOSTNAME),
			"-metadata", ("gifuu_proverb=" + tools.MACHINE_PROVERB),
			PathStandard,

			// Export: Alpha
			"-map", "[v3o]",
			"-c:v" /*----------*/, ternary(MediaSticker, IMAGE_ENCODE_CODEC, VIDEO_ENCODE_CODEC),
			"-preset" /*-------*/, ternary(MediaSticker, IMAGE_ENCODE_EFFORT, VIDEO_ENCODE_EFFORT),
			"-qp" /*-----------*/, ternary(MediaSticker, IMAGE_ENCODE_QUALITY, VIDEO_LARGE_QUALITY),
			"-g" /*------------*/, strconv.Itoa(VIDEO_LARGE_KEYINT),
			"-pix_fmt" /*------*/, MEDIA_COLOR_SPACE,
			"-color_range" /*--*/, MEDIA_COLOR_RANGE,
			"-svtav1-params" /**/, IMAGE_ENCODE_PARAMS,
			"-frames:v" /*-----*/, ternary(MediaSticker, "1", strconv.Itoa(VIDEO_LARGE_MAX_FRAMES)),
			"-loop" /*---------*/, ternary(MediaSticker, "1", "0"),
			"-map_metadata" /*-*/, "-1",
			"-metadata", ("gifuu_encoder=" + tools.MACHINE_HOSTNAME),
			"-metadata", ("gifuu_proverb=" + tools.MACHINE_PROVERB),
			PathAlpha,

			// Export: Moderation
			"-map", "[v4o]",
			"-f" /*-------*/, "rawvideo",
			"-fps_mode" /**/, "vfr",
			"-pix_fmt" /*-*/, "rgb24",
			"-",
		)
		cmd.Stderr = &stderr

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			tools.SendServerError(w, r, err)
			return
		}

		if err := cmd.Start(); err != nil {
			tools.SendServerError(w, r, err)
			return
		}

		// --- Auto Moderation ---
		var (
			classifyError    error
			classifyPercent  float32
			classifyAllowed  = true
			classifyComplete = make(chan struct{}, 1)
			frameSize        = (tools.MODEL_SIZE * tools.MODEL_SIZE * 3)
			tensorSize       = (tools.MODEL_FRAMERATE * frameSize)
			tensorData       = make([]float32, tensorSize)
			frameData        = make([]byte, tensorSize)
		)
		go func() {
			defer close(classifyComplete)
			frameIndex := 0
			for {
				// Process Raw Frames for Model
				n, err := io.ReadFull(stdout, frameData)
				if err == io.EOF {
					break
				}
				if err != nil && err != io.ErrUnexpectedEOF {
					cancel()
					return
				}

				frameCount := n / frameSize
				for i := 0; i < n; i++ {
					tensorData[i] = float32(frameData[i]) / 255.0
				}

				// Classify Frames
				logits, err := tools.ModelClassifyTensorBatch(
					tensorData[:frameCount*frameSize],
					frameCount,
				)
				if err != nil {
					classifyError = err
					cancel()
					return
				}

				// Calculate Results
				for idx, results := range logits {
					classifyPercent = (results.Hentai + results.Porn + (results.Sexy * 0.9))
					classifyAllowed = (classifyPercent < tools.MODEL_THRESHOLD_DENY)

					fmt.Fprintf(TempLogger,
						"#%02d | D: %.2f | H: %.2f | N: %.2f | P: %.2f | S: %.2f | T: %.2f%% | OK: %t\n",
						frameIndex+idx,
						results.Drawing,
						results.Hentai,
						results.Neutral,
						results.Porn,
						results.Sexy,
						classifyPercent,
						classifyAllowed,
					)

					if classifyPercent > MediaRating {
						MediaRating = classifyPercent
					}

					if !classifyAllowed {
						cancel()
						return
					}
				}
				frameIndex += frameCount

			}
		}()

		// --- Wait for Results ---
		err = cmd.Wait()
		<-classifyComplete

		fmt.Fprintf(TempLogger, "\n%s\nProcessing completed in %s\n",
			stderr.String(),
			time.Since(t),
		)

		if !classifyAllowed {
			tools.SendClientError(w, r, tools.ERROR_MEDIA_INAPPROPRIATE)
			return
		}
		if classifyError != nil {
			tools.SendServerError(w, r, classifyError)
			return
		}
		if err != nil {
			tools.SendServerError(w, r, err)
			return
		}
	}

	// --------------------------------------------------------------------------------
	// --- [ Upload Objects ] ---------------------------------------------------------
	// --------------------------------------------------------------------------------
	{
		// --- Generate Object Keys ---
		var (
			keyAlpha    = path.Join(TempIDString, MEDIA_FILENAME_ALPHA)
			keyPreview  = path.Join(TempIDString, MEDIA_FILENAME_PREVIEW)
			keyStandard = path.Join(TempIDString, MEDIA_FILENAME_STANDARD)
		)
		defer func() {
			if !TempSuccess {
				ctx, cancel := context.WithTimeout(context.Background(), tools.TIMEOUT_CONTEXT)
				defer cancel()
				if err := tools.Storage.Delete(ctx, keyPreview, keyStandard, keyAlpha); err != nil {
					tools.LoggerStorage.Log(tools.WARN, "Failed to delete incomplete uploads: %s", err)
				}
			}
		}()

		// --- Copy Completed Files ---
		for _, op := range []struct {
			SourceFilename string
			DestinationKey string
			ContentType    string
		}{
			{PathAlpha, keyAlpha, "video/webm"},
			{PathPreview, keyPreview, "image/avif"},
			{PathStandard, keyStandard, "image/avif"},
		} {
			// Open File for Reading
			f, err := os.Open(op.SourceFilename)
			if err != nil {
				tools.SendServerError(w, r, err)
				return
			}

			// Upload Image to Object Storage
			err = tools.Storage.Put(ctx, op.DestinationKey, op.ContentType, f)
			if err != nil {
				f.Close()
				tools.SendServerError(w, r, err)
				return
			}
			f.Close()

		}
	}

	// --------------------------------------------------------------------------------
	// --- [ Upload Metadata ] --------------------------------------------------------
	// --------------------------------------------------------------------------------
	{
		// --- Begin Transaction ---
		tx, err := tools.Database.Begin(ctx)
		if err != nil {
			tools.SendServerError(w, r, err)
			return
		}
		defer tx.Rollback(ctx)

		// --- Insert Row ---
		if _, err = tx.Exec(ctx,
			`INSERT INTO gifuu.upload (
				id,
				upload_address,
				upload_token,
				sticker,
				width,
				height,
				rating,
				title
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			TempID,
			ClientAddress,
			tools.GenerateHash(ClientToken),
			MediaSticker,
			MediaWidth,
			MediaHeight,
			MediaRating,
			Body.Title,
		); err != nil {
			tools.SendServerError(w, r, err)
			return
		}

		// --- Upsert Tags ---
		if _, err := tx.Exec(ctx,
			`WITH upserted_tags AS (
				INSERT INTO gifuu.tag (label)
				SELECT unnest($1::text[])
				ON CONFLICT (label) DO UPDATE SET label = EXCLUDED.label
				RETURNING id
			)
			INSERT INTO gifuu.upload_tag (gif_id, tag_id)
			SELECT $2, id FROM upserted_tags`,
			Body.Tags,
			TempID,
		); err != nil {
			tools.SendServerError(w, r, err)
			return
		}

		// --- Commit Transaction ---
		if err := tx.Commit(ctx); err != nil {
			tools.SendServerError(w, r, err)
			return
		}
	}

	// --------------------------------------------------------------------------------
	// --- [ Return Results ] ---------------------------------------------------------
	// --------------------------------------------------------------------------------
	TempSuccess = true
	tools.SendJSON(w, r, http.StatusOK, map[string]any{
		"id":         strconv.FormatInt(TempID, 10),
		"edit_token": ClientToken,
	})
}
