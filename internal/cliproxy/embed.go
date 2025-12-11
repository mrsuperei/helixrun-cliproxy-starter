package cliproxy

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	cliproxysdk "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"

	// Register all built-in request/response translators (OpenAI, Gemini, etc.).
	_ "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator/builtin"
)

// StartOptions describes how the embedded CLIProxy service should be launched.
type StartOptions struct {
	// ConfigPath points to the CLIProxy configuration file.
	ConfigPath string
	// CoreManager allows callers to inject a custom auth manager (for DB-backed stores).
	CoreManager *coreauth.Manager
	// LocalManagementPassword enforces a password only accepted from localhost callers.
	LocalManagementPassword string
}

// Service wraps the embedded CLIProxyAPI service instance.
type Service struct {
	svc     *cliproxysdk.Service
	manager *coreauth.Manager
}

// Start creates and runs an embedded CLIProxyAPI Service using the provided options.
// The service runs in a background goroutine and stops when the supplied context is cancelled.
func Start(ctx context.Context, opts StartOptions) (*Service, error) {
	configPath := strings.TrimSpace(opts.ConfigPath)
	if configPath == "" {
		return nil, fmt.Errorf("config path is required")
	}
	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return nil, fmt.Errorf("resolve config path: %w", err)
	}

	if _, err := os.Stat(absPath); err != nil {
		return nil, fmt.Errorf("config file %q not found or unreadable: %w", absPath, err)
	}

	cfg, err := cliproxysdk.LoadConfig(absPath)
	if err != nil {
		return nil, fmt.Errorf("load cliproxy config: %w", err)
	}

	builder := cliproxysdk.NewBuilder().
		WithConfig(cfg).
		WithConfigPath(absPath)
	if opts.CoreManager != nil {
		builder = builder.WithCoreAuthManager(opts.CoreManager)
	}
	if opts.LocalManagementPassword != "" {
		builder = builder.WithLocalManagementPassword(opts.LocalManagementPassword)
	}
	svc, err := builder.Build()
	if err != nil {
		return nil, fmt.Errorf("build cliproxy service: %w", err)
	}

	go func() {
		if err := svc.Run(ctx); err != nil && ctx.Err() == nil {
			log.Printf("cliproxy service stopped with error: %v", err)
		}
	}()

	return &Service{svc: svc, manager: opts.CoreManager}, nil
}

// Shutdown gracefully stops the embedded CLIProxyAPI service.
func (s *Service) Shutdown(ctx context.Context) error {
	if s == nil || s.svc == nil {
		return nil
	}
	return s.svc.Shutdown(ctx)
}

// Manager returns the shared core auth manager, if one was provided.
func (s *Service) Manager() *coreauth.Manager {
	if s == nil {
		return nil
	}
	return s.manager
}
