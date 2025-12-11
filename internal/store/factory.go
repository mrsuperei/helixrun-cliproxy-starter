package authstore

import (
	"context"
	"log"
	"os"
	"strings"

	authrepo "helixrun-cliproxy-starter/internal/cliproxy/repo/auth"
)

const defaultDSN = "postgres://helixrun:test@localhost:5432/helixrun?sslmode=disable"

// FromEnv builds an auth store using HELIXRUN_DB_DSN (optional) and the provided auth directory.
func FromEnv(ctx context.Context, authDir string) (*authrepo.Store, error) {
	dsn := strings.TrimSpace(os.Getenv("HELIXRUN_DB_DSN"))
	if dsn == "" {
		dsn = defaultDSN
		log.Printf("HELIXRUN_DB_DSN not set; defaulting to %s", dsn)
	}
	return authrepo.New(ctx, authrepo.Config{
		DSN:     dsn,
		AuthDir: authDir,
	})
}
