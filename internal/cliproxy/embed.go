package cliproxy

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	sdkAuth "github.com/router-for-me/CLIProxyAPI/v6/sdk/auth"
	cliproxysdk "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy"

	// Register all built-in request/response translators (OpenAI, Gemini, etc.).
	_ "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator/builtin"

	authstore "helixrun-cliproxy-starter/internal/store"
)

// StartOptions describes how the embedded CLIProxy service should be launched.
type StartOptions struct {
	// ConfigPath points to the CLIProxy configuration file.
	ConfigPath string
	// LocalManagementPassword enforces a password only accepted from localhost callers.
	LocalManagementPassword string
}

// Service wraps the embedded CLIProxyAPI service instance.
type Service struct {
	svc *cliproxysdk.Service
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

	// Optional: configure official Postgres-backed auth/token store when PGSTORE_DSN is set.
	if dsn := firstNonEmptyEnv("PGSTORE_DSN", "pgstore_dsn"); dsn != "" {
		schema := firstNonEmptyEnv("PGSTORE_SCHEMA", "pgstore_schema")
		spoolDir := firstNonEmptyEnv("PGSTORE_LOCAL_PATH", "pgstore_local_path")

		store, err := authstore.NewPostgresTokenStore(ctx, authstore.PostgresTokenConfig{
			DSN:      dsn,
			Schema:   schema,
			SpoolDir: spoolDir,
		})
		if err != nil {
			return nil, fmt.Errorf("init postgres token store: %w", err)
		}

		if err := store.EnsureSchema(ctx); err != nil {
			return nil, fmt.Errorf("ensure postgres token schema: %w", err)
		}
		if err := store.SyncFromDatabase(ctx); err != nil {
			return nil, fmt.Errorf("sync auth from postgres: %w", err)
		}

		// Make CLIProxy watch the mirrored auth directory and use Postgres as token store.
		cfg.AuthDir = store.AuthDir()
		sdkAuth.RegisterTokenStore(store)
	}

	builder := cliproxysdk.NewBuilder().
		WithConfig(cfg).
		WithConfigPath(absPath)
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

	return &Service{svc: svc}, nil
}

// Shutdown gracefully stops the embedded CLIProxyAPI service.
func (s *Service) Shutdown(ctx context.Context) error {
	if s == nil || s.svc == nil {
		return nil
	}
	return s.svc.Shutdown(ctx)
}

func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		if val := strings.TrimSpace(os.Getenv(key)); val != "" {
			return val
		}
	}
	return ""
}
