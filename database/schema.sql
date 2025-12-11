-- HelixRun credential storage schema
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
