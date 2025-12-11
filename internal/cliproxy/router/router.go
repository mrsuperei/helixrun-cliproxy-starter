package router

import (
	"context"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

// Server proxies /cliproxy requests and exposes HelixRun admin endpoints.
type Server struct {
	srv *http.Server
}

// New constructs a server using the provided dependencies.
func New(addr string, cliproxyBase *url.URL, managementKey string) *Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Serve static admin UI assets (management.html, etc.).
	mux.Handle("/admin/", http.StripPrefix("/admin/", http.FileServer(http.Dir("./config/static"))))

	proxy := httputil.NewSingleHostReverseProxy(cliproxyBase)
	mux.Handle("/cliproxy/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if managementKey != "" {
			path := strings.TrimPrefix(r.URL.Path, "/cliproxy")
			if path == "" {
				path = "/"
			}
			if strings.HasPrefix(path, "/v0/management") || strings.HasPrefix(path, "/management") {
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

// Start begins serving HTTP traffic.
func (s *Server) Start() error {
	return s.srv.ListenAndServe()
}

// Shutdown attempts a graceful stop.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

// Addr returns listening address.
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
