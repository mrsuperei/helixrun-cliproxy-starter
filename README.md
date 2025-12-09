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
> Put a plaintext `MANAGEMENT_PASSWORD=...` entry in `.env` (auto-loaded on
> startup) or export it in your shell before running `go run ./cmd/server`.
> The HTTP proxy uses this value to forward management requests and the
> embedded CLIProxyAPI runtime also treats it as the remote management password.
> If you omit it, the proxy falls back to the hashed key from the YAML file,
> which the management API will reject.

## Layout

- `cmd/server/main.go`  
  Entry point. Starts:
  - embedded CLIProxyAPI service using `config/cliproxy.yaml`
  - HelixRun HTTP server on `:8080` that proxies `/cliproxy/*` to `127.0.0.1:8317`.

- `internal/cliproxyembed`  
  Helper to construct and run `cliproxy.Service` via `cliproxy.NewBuilder()`.

- `internal/httpserver`  
  Thin wrapper around `net/http` with a reverse proxy based on
  `httputil.NewSingleHostReverseProxy`.

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
