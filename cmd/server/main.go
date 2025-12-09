package main

import (
	"bufio"
	"context"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	cliproxy "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy"

	"helixrun-cliproxy-starter/internal/cliproxyembed"
	"helixrun-cliproxy-starter/internal/httpserver"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle SIGINT/SIGTERM for graceful shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("received shutdown signal")
		cancel()
	}()

	// Path to CLIProxyAPI config file
	configPath := "./config/cliproxy.yaml"

	if err := loadDotEnv(".env"); err != nil {
		log.Printf("warning: failed loading .env file: %v", err)
	}

	cfg, err := cliproxy.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("failed to load cliproxy config: %v", err)
	}

	managementKey := strings.TrimSpace(os.Getenv("MANAGEMENT_PASSWORD"))
	if managementKey == "" {
		managementKey = cfg.RemoteManagement.SecretKey
		if managementKey != "" && strings.HasPrefix(managementKey, "$2") {
			log.Println("warning: MANAGEMENT_PASSWORD not set; proxy will inject hashed management secret which remote API will reject")
		}
		if managementKey == "" {
			log.Println("warning: no management key configured; management endpoints require manual headers")
		}
	}

	// Start embedded CLIProxyAPI service
	cpSvc, err := cliproxyembed.Start(ctx, configPath)
	if err != nil {
		log.Fatalf("failed to start embedded CLIProxyAPI: %v", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := cpSvc.Shutdown(shutdownCtx); err != nil {
			log.Printf("error shutting down CLIProxyAPI: %v", err)
		}
	}()

	// Reverse proxy from HelixRun public HTTP server to local CLIProxyAPI
	cliproxyBase, err := url.Parse("http://127.0.0.1:8317")
	if err != nil {
		log.Fatalf("invalid cliproxy base URL: %v", err)
	}

	httpSrv := httpserver.New(":8080", cliproxyBase, managementKey)

	go func() {
		log.Printf("HelixRun public server listening on %s (proxying to %s)", httpSrv.Addr(), cliproxyBase.String())
		if err := httpSrv.Start(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("context cancelled, shutting down servers")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Printf("error shutting down HelixRun HTTP server: %v", err)
	}
}

func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.Index(line, "="); idx != -1 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			val = strings.Trim(val, `"'`)
			if key != "" {
				_ = os.Setenv(key, val)
			}
		}
	}
	return scanner.Err()
}
