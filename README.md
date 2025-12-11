# HelixRun + CLIProxyAPI Starter

This starter project embeds **CLIProxyAPI** inside a Go process and exposes all
its endpoints via a public **HelixRun** HTTP server with reverse proxying.

## Topology

```text
client (CLI / browser / HelixRun UI)
   |
   v
HelixRun HTTP server  :8080
   |   - /healthz
   |   - /cliproxy/*  --> reverse proxy
   v
CLIProxyAPI (embedded) :8317
   - /v1/models
   - /v1/chat/completions
   - /v0/management/*
   - /management.html
```

All CLIProxyAPI endpoints are only bound to `127.0.0.1:8317` and not exposed
directly. External traffic hits `:8080` and is forwarded to `/cliproxy/*`.

Remote management (EasyCLI or the Web UI / Management Center) connects to
`http://YOUR_PUBLIC_HOST:8080/cliproxy` using the `remote-management.secret-key`
from `config/cliproxy.yaml`.

> **Management key injection**  
> Set `LOCAL_MANAGEMENT_PASSWORD=...` (preferred) or the legacy
> `MANAGEMENT_PASSWORD=...` entry in `.env`. The HTTP proxy now injects this
> plaintext value into every `/v0/management` request so the hashed secret from
> `config/cliproxy.yaml` never traverses the proxy. If both variables are empty
> the proxy falls back to the hashed secret, but the CLIProxy management API will
> reject that value.

## Credential database

HelixRun now persists provider credentials in PostgreSQL (while still mirroring
JSON to `auths/` so CLIProxy's watcher continues to work). Apply the schema from
`database/schema.sql` once per database:

```bash
psql "$HELIXRUN_DB_DSN" -f database/schema.sql
```

Environment variables:

- `HELIXRUN_DB_DSN` – PostgreSQL connection string. Defaults to
  `postgres://helixrun:helixrun@localhost:5432/helixrun?sslmode=disable`.
- `HELIXRUN_DB_SCHEMA` is not required; the schema file always creates/uses the
  `provider_credentials` table in the connected database.

The repository implements the CLIProxy SDK token store and mirrors auth files to
`auths/`, so CLIProxy hot-reload behaviour remains unchanged.

### Credential API

The HelixRun HTTP server exposes a small admin API for managing credentials
without touching the filesystem. All endpoints require the local management
password via the `Authorization: Bearer ...` or `X-Management-Key` header. A
full reference is available in `endpoints.md`.

| Method | Path                    | Description                                  |
|--------|------------------------|----------------------------------------------|
| GET    | `/api/credentials`     | List all stored provider credentials         |
| POST   | `/api/credentials`     | Create/import a credential (JSON payload)    |
| GET    | `/api/credentials/{id}`| Fetch a single credential                    |
| DELETE | `/api/credentials/{id}`| Remove a credential and disable it in runtime|

Example create payload:

```json
{
  "provider": "gemini",
  "label": "dev api key",
  "attributes": {
    "api_key": "sk-..."
  },
  "metadata": {
    "type": "gemini",
    "project_id": "gemini-dev"
  }
}
```

The server automatically supplies an `id` when omitted (`{provider}-{uuid}.json`)
and ensures the `metadata.type` matches the provider so CLIProxy can route the
credential correctly.

## Layout

- `cmd/server/main.go`  
  Entry point. Starts:
  - embedded CLIProxyAPI service using `config/cliproxy.yaml`
  - HelixRun HTTP server on `:8080` that proxies `/cliproxy/*` to `127.0.0.1:8317`.

- `internal/cliproxy`  
  Helpers around the embedded `cliproxy.Service` lifecycle.

- `internal/repo/credentials`  
  PostgreSQL-backed credential repository that also implements the CLIProxy token
  store interface.

- `internal/handler/credentials`  
  HTTP handlers for `/api/credentials` CRUD endpoints.

- `internal/router`  
  Shared HTTP server wiring that exposes health, credential API and the
  `/cliproxy/*` reverse proxy.

- `config/cliproxy.yaml`  
  Minimal configuration with:
  - `remote-management.allow-remote: true`
  - `remote-management.secret-key: CHANGE_ME_REMOTE_SECRET`
  - development `api-keys` entry.

## Running locally

```bash
cd helixrun-cliproxy-starter

# Adjust config/cliproxy.yaml as needed, then:
go mod tidy
go run ./cmd/server
```

You should see logs indicating:

- CLIProxyAPI has started on `:8317`
- HelixRun server is listening on `:8080`

### Quick tests

1. Health check (HelixRun):

```bash
curl http://localhost:8080/healthz
```

2. List models via proxy (OpenAI-compatible endpoint):

```bash
curl http://localhost:8080/cliproxy/v1/models \
  -H "Authorization: Bearer helixrun-dev-key"
```

3. Remote management (EasyCLI / Web UI):

- Base URL: `http://YOUR_PUBLIC_HOST:8080/cliproxy`
- Management key: value from `remote-management.secret-key` in `config/cliproxy.yaml`.

You can also open the built-in WebUI directly:

```text
http://localhost:8080/cliproxy/management.html
```

This is proxied to the embedded CLIProxyAPI instance.

4. Simple credential UI

HelixRun also exposes a minimal HTML/JS UI for managing provider credentials and starting OAuth/device
flows for supported providers:

```text
http://localhost:8080/admin/credentials.html
```

This page:

- Talks to the HelixRun credential API at `/api/credentials` using the local management password you
  enter in the UI.
- Delegates provider logins to the embedded CLIProxyAPI management endpoints via `/cliproxy/v0/management`,
  so successful OAuth/device flows create auth files that are automatically mirrored into the
  `provider_credentials` table and show up in the credential list.
