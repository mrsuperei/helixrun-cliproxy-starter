package httpserver

import (
	"context"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

// Server wraps the public HelixRun HTTP server that proxies requests
// to the local CLIProxyAPI instance.
type Server struct {
	srv *http.Server
}

// New constructs a new Server instance listening on addr, proxying all
// /cliproxy/* requests to the given cliproxyBase URL.
//
// Example:
//
//	addr := ":8080"
//	base, _ := url.Parse("http://127.0.0.1:8317")
//	s := httpserver.New(addr, base)
func New(addr string, cliproxyBase *url.URL, managementKey string) *Server {
	mux := http.NewServeMux()

	// Simple health endpoint for uptime checks.
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok")) // best-effort
	})

	// Reverse proxy: /cliproxy/* -> CLIProxyAPI (strip prefix).
	proxy := httputil.NewSingleHostReverseProxy(cliproxyBase)

	mux.Handle("/cliproxy/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if managementKey != "" {
			path := r.URL.Path
			if strings.HasPrefix(path, "/cliproxy") {
				path = strings.TrimPrefix(path, "/cliproxy")
				if path == "" {
					path = "/"
				}
			}
			if strings.HasPrefix(path, "/v0/management") || strings.HasPrefix(path, "/management") {
				// Inject X-Management-Key so callers do not need to provide it manually.
				r.Header.Set("X-Management-Key", managementKey)
			}
		}
		http.StripPrefix("/cliproxy", proxy).ServeHTTP(w, r)
	}))

	srv := &http.Server{
		Addr:         addr,
		Handler:      loggingMiddleware(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	return &Server{srv: srv}
}

// Start starts the HTTP server and blocks until it stops.
func (s *Server) Start() error {
	return s.srv.ListenAndServe()
}

// Shutdown gracefully shuts down the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

// Addr returns the listen address of the underlying http.Server.
func (s *Server) Addr() string {
	return s.srv.Addr
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s from %s in %s", r.Method, r.URL.Path, r.RemoteAddr, time.Since(start))
	})
}
