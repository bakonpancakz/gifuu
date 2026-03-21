package main

import (
	"context"
	"gifuu/routes"
	"gifuu/tools"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

func main() {
	time.Local = time.UTC

	// Startup Services
	var stopCtx, stop = context.WithCancel(context.Background())
	var stopWg sync.WaitGroup
	var syncWg sync.WaitGroup

	tools.LoggerInit.Log(tools.INFO, "Starting Services")
	for _, fn := range []func(stop context.Context, await *sync.WaitGroup){
		tools.SetupDatabase,
		tools.SetupStorage,
		tools.SetupModel,
	} {
		syncWg.Add(1)
		go func() {
			defer syncWg.Done()
			fn(stopCtx, &stopWg)
		}()
	}
	syncWg.Wait()

	go StartupHTTP(stopCtx, &stopWg)

	// Await Shutdown Signal
	cancel := make(chan os.Signal, 1)
	signal.Notify(cancel, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-cancel
	stop()

	// Begin Shutdown Process
	tools.LoggerInit.Log(tools.WARN, "Shutting Down!")
	timeout, finish := context.WithTimeout(context.Background(), tools.TIMEOUT_CONTEXT)
	defer finish()
	go func() {
		<-timeout.Done()
		if timeout.Err() == context.DeadlineExceeded {
			log.Fatalln("[MAIN] Shutdown Deadline Exceeded")
		}
	}()
	stopWg.Wait()
	os.Exit(0)
}

func SetupMux() *http.ServeMux {

	var (
		mux                      = http.NewServeMux()
		ratelimitAnimationRead   = tools.NewRatelimit("ANIMATION_READ", 300, 1*time.Minute)
		ratelimitAnimationWrite  = tools.NewRatelimit("ANIMATION_WRITE", 3, time.Minute)
		ratelimitAnimationDelete = tools.NewRatelimit("ANIMATION_DELETE", 20, time.Minute)
		ratelimitAnimationQuery  = tools.NewRatelimit("ANIMATION_QUERY", 60, time.Minute)
		ratelimitTagAutocomplete = tools.NewRatelimit("TAG_AUTOCOMPLETE", 60, time.Minute)
		ratelimitTagLeaderboard  = tools.NewRatelimit("TAG_LEADERBOARD", 60, time.Minute)
		ratelimitPoWCreate       = tools.NewRatelimit("POW_CREATE", 60, time.Minute)
	)

	// --- Backend ---
	mux.Handle("/tags/popular", tools.MethodHandler{
		http.MethodPost: tools.Chain(routes.POST_Tags_Popular, ratelimitTagLeaderboard),
	})
	mux.Handle("/tags/autocomplete", tools.MethodHandler{
		http.MethodPost: tools.Chain(routes.POST_Tags_Autocomplete, ratelimitTagAutocomplete),
	})
	mux.Handle("/animations", tools.MethodHandler{
		http.MethodPost: tools.Chain(routes.POST_Animations, ratelimitAnimationWrite, tools.RequirePoW),
	})
	mux.Handle("/animations/latest", tools.MethodHandler{
		http.MethodPost: tools.Chain(routes.POST_Animations_Latest, ratelimitAnimationQuery),
	})
	mux.Handle("/animations/search", tools.MethodHandler{
		http.MethodPost: tools.Chain(routes.POST_Animations_Search, ratelimitAnimationQuery),
	})
	mux.Handle("/animations/{id}", tools.MethodHandler{
		http.MethodGet:    tools.Chain(routes.GET_Animations_ID, ratelimitAnimationRead),
		http.MethodDelete: tools.Chain(routes.DELETE_Animations_ID, ratelimitAnimationDelete),
	})
	mux.Handle("/pow", tools.MethodHandler{
		http.MethodGet: tools.Chain(routes.GET_PoW, ratelimitPoWCreate),
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		tools.SendClientError(w, r, tools.ERROR_UNKNOWN_ENDPOINT)
	})

	return mux
}

func StartupHTTP(stop context.Context, await *sync.WaitGroup) {

	svr := http.Server{
		Handler:           SetupMux(),
		Addr:              tools.HTTP_ADDRESS,
		MaxHeaderBytes:    4096,
		IdleTimeout:       10 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      600 * time.Second,
		ReadTimeout:       120 * time.Second,
	}

	// Shutdown Logic
	await.Add(1)
	go func() {
		defer await.Done()
		<-stop.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), tools.TIMEOUT_SHUTDOWN)
		defer cancel()

		if err := svr.Shutdown(shutdownCtx); err != nil {
			tools.LoggerHTTP.Log(tools.ERROR, "Shutdown error: %s", err)
		}

		tools.LoggerHTTP.Log(tools.INFO, "Closed")
	}()

	// Startup Logic
	tools.LoggerHTTP.Log(tools.INFO, "Listening @ %s", svr.Addr)
	if err := svr.ListenAndServe(); err != http.ErrServerClosed {
		tools.LoggerHTTP.Log(tools.FATAL, "Startup Failed: %s", err)
	}
}
