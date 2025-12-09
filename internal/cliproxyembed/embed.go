package cliproxyembed

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	cliproxy "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy"

	// Register all built-in request/response translators (OpenAI ↔ Gemini, etc.).

	_ "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator/builtin"
)

// Service wraps the embedded CLIProxyAPI service instance.
type Service struct {
	svc *cliproxy.Service
}

// Start creates and runs an embedded CLIProxyAPI Service using the given
// configuration file path. The service runs in a background goroutine and
// stops when the provided context is cancelled.
func Start(ctx context.Context, configPath string) (*Service, error) {
	// Altijd een absoluut pad gebruiken; de SDK verwacht dit.
	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return nil, fmt.Errorf("resolve config path: %w", err)
	}

	// Optioneel, maar handig: check of het bestand bestaat en leesbaar is.
	if _, err := os.Stat(absPath); err != nil {
		return nil, fmt.Errorf("config file %q not found or unreadable: %w", absPath, err)
	}

	cfg, err := cliproxy.LoadConfig(absPath)
	if err != nil {
		return nil, fmt.Errorf("load cliproxy config: %w", err)
	}

	// Bouw de embedded CLIProxy service met dezelfde YAML-configuratie als de CLI.
	builder := cliproxy.NewBuilder().
		WithConfig(cfg).
		WithConfigPath(absPath)
	svc, err := builder.Build()
	if err != nil {
		return nil, fmt.Errorf("build cliproxy service: %w", err)
	}

	// Service in de background draaien; Run blokkeert normaal.
	go func() {
		if err := svc.Run(ctx); err != nil && ctx.Err() == nil {
			log.Printf("cliproxy service stopped with error: %v", err)
		}
	}()

	return &Service{svc: svc}, nil
}

// Shutdown gracefully stops the embedded CLIProxyAPI service.
func (s *Service) Shutdown(ctx context.Context) error {
	if s == nil || s.svc == nil {
		return nil
	}
	return s.svc.Shutdown(ctx)
}
