# HelixRun + CLIProxyAPI Starter

This starter project embeds **CLIProxyAPI-Extended** inside a Go process and exposes all
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
> `MANAGEMENT_PASSWORD=...` entry in `.env`. The HTTP proxy injects this
> plaintext value into every `/v0/management` request so the hashed secret from
> `config/cliproxy.yaml` never traverses the proxy. If both variables are empty
> the proxy falls back to the hashed secret, but the CLIProxy management API will
> reject that value.

## PostgreSQL-backed configuration and token store

This starter uses the **official** PostgreSQL-backed configuration and token
store provided by CLIProxyAPI / CLIProxyAPI-Extended. Configuration and
authentication data are stored in PostgreSQL and mirrored to the local
filesystem so existing `auth-dir` and watcher logic keep working unchanged.

The integration is controlled entirely via environment variables understood
by CLIProxy itself:

- `PGSTORE_DSN` (required)  
  PostgreSQL connection string, for example:  
  `postgresql://user:pass@host:5432/dbname`.
- `PGSTORE_SCHEMA` (optional, default `public`)  
  Schema where the `config_store` and `auth_store` tables are created.
- `PGSTORE_LOCAL_PATH` (optional, default current working directory)  
  Base directory for the local mirror. CLIProxy writes to
  `<PGSTORE_LOCAL_PATH or CWD>/pgstore`, mirroring `config/config.yaml`
  and the `auths/` directory.

On startup, the embedded CLIProxy service:

1. Connects to PostgreSQL using `PGSTORE_DSN` and ensures the schema exists.
2. Creates or migrates the `config_store` / `auth_store` tables as needed.
3. Maintains a writable mirror under `pgstore/` so the management API,
   Web UI and file watchers behave exactly as in file-backed mode.

No HelixRun-specific database code is required; HelixRun simply embeds
CLIProxy and forwards `/cliproxy/*` traffic.

## Layout

- `cmd/server/main.go`  
  Entry point. Starts:
  - embedded CLIProxyAPI service using `config/cliproxy.yaml`
  - HelixRun HTTP server on `:8080` that proxies `/cliproxy/*` to `127.0.0.1:8317`.

- `internal/cliproxy`  
  Helpers around the embedded `cliproxy.Service` lifecycle.

- `internal/cliproxy/router`  
  Shared HTTP server wiring that exposes health checks and the `/cliproxy/*`
  reverse proxy (including the CLIProxy management API and Web UI).

- `config/cliproxy.yaml`  
  Minimal configuration with:
  - `remote-management.allow-remote: true`
  - `remote-management.secret-key: CHANGE_ME_REMOTE_SECRET`
  - development `api-keys` entry.

## Running locally

```bash
cd helixrun-cliproxy-starter

# Adjust config/cliproxy.yaml and set PGSTORE_* env vars as needed, then:
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

You can also open the built-in management WebUI directly:

```text
http://localhost:8080/cliproxy/management.html
```

This is proxied to the embedded CLIProxyAPI instance.

