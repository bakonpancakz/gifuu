package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"
)

type BackendTag struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Usage int    `json:"usage"`
}

type BackendAnimation struct {
	ID      string       `json:"id"`
	Created string       `json:"created"`
	Width   int          `json:"width"`
	Height  int          `json:"height"`
	Rating  float64      `json:"rating"`
	Sticker bool         `json:"sticker"`
	Title   string       `json:"title"`
	Tags    []BackendTag `json:"tags"`
}

var (
	//go:embed animation.html
	tmplAnimationTemplate string
	tmplAnimation         = template.Must(template.New("").Parse(tmplAnimationTemplate))
	cacheLocal            sync.Map

	HTTP_ADDRESS     = envvar("HTTP_ADDRESS", "127.0.0.1:9000")
	HTTP_API_TOKEN   = envvar("HTTP_API_TOKEN", "")
	HTTP_API_ADDRESS = envvar("HTTP_API_ADDRESS", "https://api.gifuu.panca.kz")
	BASE_WEB         = envvar("BASE_WEB", "https://gifuu.panca.kz")
	BASE_CDN         = envvar("BASE_CDN", "https://cdn.gifuu.panca.kz")
	BASE_API         = envvar("BASE_API", "https://api.gifuu.panca.kz")
)

func envvar(field, initial string) string {
	if value := os.Getenv(field); value == "" {
		return initial
	} else {
		return value
	}
}

func init() {
	go func() {
		for {
			time.Sleep(time.Hour)
			cacheLocal.Clear()
			log.Println("Cache Cleared")
		}
	}()
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/animations/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		// --- Check Cache ---
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			http.Error(w, "Invalid ID", http.StatusBadRequest)
			return
		}

		if data, ok := cacheLocal.Load(id); ok {
			w.Header().Set("Content-Type", "text/html")
			w.Header().Set("X-Cache-Hit", "true")
			w.Write(data.([]byte))
			return
		}

		// --- Fetch Animation Info ---
		uri := fmt.Sprintf("%s/animations/%d", HTTP_API_ADDRESS, id)
		req, _ := http.NewRequestWithContext(r.Context(), "GET", uri, http.NoBody)
		req.Header.Set("X-Ratelimit-Token", HTTP_API_TOKEN)

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Println("Upstream Error:", err)
			http.Error(w, "Server Error", http.StatusServiceUnavailable)
			return
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)

		if res.StatusCode == 404 {
			http.Error(w, "Unknown Animation", http.StatusNotFound)
			return
		}
		if res.StatusCode < 200 || res.StatusCode > 299 {
			log.Println("Request Failed:", uri, string(raw))
			http.Error(w, "Server Error", http.StatusInternalServerError)
			return
		}

		// --- Render Webpage ---
		anim := BackendAnimation{}
		tags := []string{}

		if err := json.Unmarshal(raw, &anim); err != nil {
			log.Println("Invalid JSON:", uri, string(raw))
			http.Error(w, "Server Error", http.StatusInternalServerError)
			return
		}
		for _, t := range anim.Tags {
			tags = append(tags, t.Label)
		}

		buf := bytes.Buffer{}
		if err := tmplAnimation.Execute(&buf, map[string]any{
			"width":     anim.Width,
			"height":    anim.Height,
			"title":     html.EscapeString(anim.Title),
			"tags":      html.EscapeString(strings.Join(tags, ", ")),
			"uri_embed": fmt.Sprintf("%s/embed.html?id=%s&quality=standard", BASE_WEB, anim.ID),
			"uri_image": fmt.Sprintf("%s/%s/standard.avif", BASE_CDN, anim.ID),
			"uri_site":  fmt.Sprintf("%s/animations/%s", BASE_WEB, anim.ID),
		}); err != nil {
			log.Println("Render Error:", uri, err)
			http.Error(w, "Server Error", http.StatusInternalServerError)
			return
		}

		// --- Return Results ---
		cacheLocal.Store(id, buf.Bytes())
		w.Header().Add("Content-Type", "text/html")
		w.Header().Set("X-Cache-Hit", "false")
		w.Write(buf.Bytes())
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Println(r.URL)
		http.Error(w, "Unknown Page", http.StatusBadRequest)
	})

	svr := http.Server{
		Handler:           mux,
		Addr:              HTTP_ADDRESS,
		MaxHeaderBytes:    4096,
		IdleTimeout:       5 * time.Minute,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      5 * time.Second,
	}

	log.Println("Listening:", HTTP_ADDRESS)
	if err := svr.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalln("Server Closed:", err)
	}

}
