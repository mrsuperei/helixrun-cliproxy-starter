package authrepo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
)

// Config describes the settings required to connect to PostgreSQL and mirror auth files.
type Config struct {
	DSN     string
	AuthDir string
}

// Store persists provider credentials in PostgreSQL while mirroring JSON files on disk.
type Store struct {
	db      *sql.DB
	authDir string
	mu      sync.Mutex
}

// Repository exposes the operations consumed by HTTP handlers.
type Repository interface {
	List(ctx context.Context) ([]*coreauth.Auth, error)
	Get(ctx context.Context, id string) (*coreauth.Auth, error)
	Save(ctx context.Context, auth *coreauth.Auth) (string, error)
	Delete(ctx context.Context, id string) error
}

var _ coreauth.Store = (*Store)(nil)

// New connects to PostgreSQL, ensures the schema exists, and returns a credential store.
func New(ctx context.Context, cfg Config) (*Store, error) {
	dsn := strings.TrimSpace(cfg.DSN)
	if dsn == "" {
		return nil, fmt.Errorf("auth store: DSN is required")
	}
	authDir := strings.TrimSpace(cfg.AuthDir)
	if authDir == "" {
		return nil, fmt.Errorf("auth store: auth directory is required")
	}

	absAuthDir, err := filepath.Abs(authDir)
	if err != nil {
		return nil, fmt.Errorf("auth store: resolve auth dir: %w", err)
	}
	if err := os.MkdirAll(absAuthDir, 0o755); err != nil {
		return nil, fmt.Errorf("auth store: create auth dir: %w", err)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("auth store: open database: %w", err)
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(8)
	db.SetConnMaxIdleTime(5 * time.Minute)
	db.SetConnMaxLifetime(30 * time.Minute)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("auth store: ping database: %w", err)
	}

	store := &Store{
		db:      db,
		authDir: absAuthDir,
	}
	if err := store.initSchema(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

// Close releases the underlying DB connection pool.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// AuthDir exposes the mirrored auth directory path.
func (s *Store) AuthDir() string {
	if s == nil {
		return ""
	}
	return s.authDir
}

// PersistConfig is a no-op to satisfy watcher expectations.
func (s *Store) PersistConfig(context.Context) error {
	return nil
}

// PersistAuthFiles syncs manual filesystem edits back into PostgreSQL.
func (s *Store) PersistAuthFiles(ctx context.Context, _ string, paths ...string) error {
	if s == nil || len(paths) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, raw := range paths {
		path := strings.TrimSpace(raw)
		if path == "" {
			continue
		}
		full := s.ensureAbsolute(path)
		data, err := os.ReadFile(full)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				if rel, relErr := s.relativeName(full); relErr == nil {
					if err := s.deleteRecord(ctx, rel); err != nil {
						return err
					}
				}
				continue
			}
			return fmt.Errorf("auth store: read %s: %w", full, err)
		}
		if len(data) == 0 {
			continue
		}
		var metadata map[string]any
		if err := json.Unmarshal(data, &metadata); err != nil {
			return fmt.Errorf("auth store: invalid json %s: %w", full, err)
		}
		provider := normalizeProvider(metadata["type"])
		label := preferredLabel(metadata)
		info, _ := os.Stat(full)
		mod := time.Now().UTC()
		if info != nil {
			mod = info.ModTime().UTC()
		}
		relName, err := s.relativeName(full)
		if err != nil {
			return err
		}
		auth := &coreauth.Auth{
			ID:         relName,
			Provider:   provider,
			Label:      label,
			Status:     coreauth.StatusActive,
			Attributes: map[string]string{"path": full},
			Metadata:   metadata,
			CreatedAt:  mod,
			UpdatedAt:  mod,
		}
		auth.FileName = relName
		if err := s.persistRecord(ctx, auth); err != nil {
			return err
		}
	}
	return nil
}

// List returns all credentials tracked in PostgreSQL.
func (s *Store) List(ctx context.Context) ([]*coreauth.Auth, error) {
	if s == nil {
		return nil, fmt.Errorf("auth store: not initialised")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT payload, file_name
		FROM provider_credentials
		ORDER BY provider, id`)
	if err != nil {
		return nil, fmt.Errorf("auth store: list rows: %w", err)
	}
	defer rows.Close()

	auths := make([]*coreauth.Auth, 0, 32)
	for rows.Next() {
		var (
			raw      []byte
			fileName string
		)
		if err := rows.Scan(&raw, &fileName); err != nil {
			return nil, fmt.Errorf("auth store: scan row: %w", err)
		}
		var auth coreauth.Auth
		if err := json.Unmarshal(raw, &auth); err != nil {
			return nil, fmt.Errorf("auth store: decode payload: %w", err)
		}
		s.applyMirrorPath(&auth, fileName)
		auths = append(auths, auth.Clone())
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("auth store: iterate rows: %w", err)
	}
	return auths, nil
}

// Get loads a single credential.
func (s *Store) Get(ctx context.Context, id string) (*coreauth.Auth, error) {
	if s == nil {
		return nil, fmt.Errorf("auth store: not initialised")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("auth store: id required")
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT payload, file_name
		FROM provider_credentials
		WHERE id = $1`, id)
	var (
		raw      []byte
		fileName string
	)
	if err := row.Scan(&raw, &fileName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("auth store: scan row: %w", err)
	}
	var auth coreauth.Auth
	if err := json.Unmarshal(raw, &auth); err != nil {
		return nil, fmt.Errorf("auth store: decode payload: %w", err)
	}
	s.applyMirrorPath(&auth, fileName)
	return auth.Clone(), nil
}

// Save upserts a credential and mirrors metadata to disk.
func (s *Store) Save(ctx context.Context, auth *coreauth.Auth) (string, error) {
	if s == nil {
		return "", fmt.Errorf("auth store: not initialised")
	}
	if auth == nil {
		return "", fmt.Errorf("auth store: auth is nil")
	}
	id := strings.TrimSpace(auth.ID)
	if id == "" {
		return "", fmt.Errorf("auth store: id required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	path, rel, err := s.resolvePath(auth)
	if err != nil {
		return "", err
	}
	if auth.Disabled {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("auth store: delete file: %w", err)
		}
		return "", s.deleteRecord(ctx, rel)
	}

	if auth.Metadata == nil {
		auth.Metadata = make(map[string]any)
	}
	if err := s.writeMetadata(path, auth.Metadata); err != nil {
		return "", err
	}

	now := time.Now().UTC()
	if auth.CreatedAt.IsZero() {
		auth.CreatedAt = now
	}
	auth.UpdatedAt = now

	// Default: OAuth/device-flow records zonder status worden "active"
	if auth.Status == "" && !auth.Disabled {
		auth.Status = coreauth.StatusActive
	}

	auth.FileName = rel
	if auth.Attributes == nil {
		auth.Attributes = make(map[string]string)
	}
	auth.Attributes["path"] = path

	if err := s.persistRecord(ctx, auth); err != nil {
		return "", err
	}
	return path, nil
}

// Delete removes a credential permanently.
func (s *Store) Delete(ctx context.Context, id string) error {
	if s == nil {
		return fmt.Errorf("auth store: not initialised")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("auth store: id required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.ensureAbsolute(id)
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("auth store: remove file: %w", err)
	}
	rel, err := s.relativeName(path)
	if err != nil {
		return err
	}
	return s.deleteRecord(ctx, rel)
}

// SetBaseDir implements the optional interface expected by CLIProxy authenticators.
func (s *Store) SetBaseDir(string) {}

func (s *Store) initSchema(ctx context.Context) error {
	if s == nil {
		return fmt.Errorf("auth store: nil receiver")
	}
	if _, err := s.db.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("auth store: ensure schema: %w", err)
	}
	return nil
}

func (s *Store) persistRecord(ctx context.Context, auth *coreauth.Auth) error {
	payload, err := json.Marshal(auth)
	if err != nil {
		return fmt.Errorf("auth store: marshal auth: %w", err)
	}
	rel := auth.FileName
	if rel == "" {
		rel = auth.ID
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO provider_credentials (id, provider, label, file_name, payload, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO UPDATE SET
			provider = EXCLUDED.provider,
			label = EXCLUDED.label,
			file_name = EXCLUDED.file_name,
			payload = EXCLUDED.payload,
			updated_at = EXCLUDED.updated_at
	`, auth.ID, strings.ToLower(strings.TrimSpace(auth.Provider)), auth.Label, rel, payload, auth.CreatedAt.UTC(), auth.UpdatedAt.UTC())
	if err != nil {
		return fmt.Errorf("auth store: upsert row: %w", err)
	}
	return nil
}

func (s *Store) deleteRecord(ctx context.Context, rel string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM provider_credentials WHERE id = $1`, rel); err != nil {
		return fmt.Errorf("auth store: delete row: %w", err)
	}
	return nil
}

func (s *Store) resolvePath(auth *coreauth.Auth) (string, string, error) {
	if auth == nil {
		return "", "", fmt.Errorf("auth store: auth is nil")
	}
	fileName := strings.TrimSpace(auth.FileName)
	if fileName == "" {
		fileName = strings.TrimSpace(auth.ID)
	}
	if fileName == "" {
		return "", "", fmt.Errorf("auth store: missing file name")
	}
	if strings.Contains(fileName, "..") {
		return "", "", fmt.Errorf("auth store: invalid relative path %s", fileName)
	}
	abs := filepath.Join(s.authDir, filepath.FromSlash(fileName))
	return abs, filepath.ToSlash(fileName), nil
}

func (s *Store) relativeName(path string) (string, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(s.authDir, path)
	}
	clean := filepath.Clean(path)
	rel, err := filepath.Rel(s.authDir, clean)
	if err != nil {
		return "", fmt.Errorf("auth store: compute relative path: %w", err)
	}
	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("auth store: path %s outside auth dir", path)
	}
	return filepath.ToSlash(rel), nil
}

func (s *Store) ensureAbsolute(path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(s.authDir, filepath.FromSlash(path))
}

func (s *Store) writeMetadata(path string, metadata map[string]any) error {
	raw, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("auth store: marshal metadata: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("auth store: create auth subdir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("auth store: write temp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("auth store: rename temp file: %w", err)
	}
	return nil
}

func (s *Store) applyMirrorPath(auth *coreauth.Auth, relName string) {
	if auth == nil {
		return
	}
	name := relName
	if name == "" {
		name = strings.TrimSpace(auth.FileName)
	}
	if name == "" {
		name = auth.ID
	}
	name = filepath.ToSlash(name)
	auth.FileName = name
	if auth.Attributes == nil {
		auth.Attributes = make(map[string]string)
	}
	auth.Attributes["path"] = filepath.Join(s.authDir, filepath.FromSlash(name))
}

func normalizeProvider(value any) string {
	if s, ok := value.(string); ok {
		if trimmed := strings.TrimSpace(s); trimmed != "" {
			return strings.ToLower(trimmed)
		}
	}
	return "unknown"
}

func preferredLabel(meta map[string]any) string {
	if meta == nil {
		return ""
	}
	for _, key := range []string{"label", "email", "project_id"} {
		if v, ok := meta[key].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

const schemaSQL = `
CREATE TABLE IF NOT EXISTS provider_credentials (
	id TEXT PRIMARY KEY,
	provider TEXT NOT NULL,
	label TEXT,
	file_name TEXT NOT NULL,
	payload JSONB NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_provider_credentials_provider
	ON provider_credentials (provider);
`
