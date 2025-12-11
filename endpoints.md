# HelixRun API Endpoints

HelixRun exposes a small HTTP surface and forwards everything else to the
embedded CLIProxyAPI instance.

## `/healthz`

Simple health check for the HelixRun HTTP server.

- **Method:** `GET`
- **Response:** `200 OK` with body `ok`

## `/cliproxy/*`

Reverse proxy in front of the embedded CLIProxyAPI-Extended server.

- **Base URL:** `http://YOUR_PUBLIC_HOST:8080/cliproxy`
- **Auth:** same as the underlying CLIProxy instance (for example
  `Authorization: Bearer <api-key>` for OpenAI-compatible endpoints).

All standard CLIProxy endpoints are available under this prefix, including:

- `/cliproxy/v1/models`
- `/cliproxy/v1/chat/completions`
- `/cliproxy/v0/management/*`
- `/cliproxy/management.html` (management WebUI)

For management traffic, HelixRun automatically injects the local management
password into `X-Management-Key` for requests to:

- `/cliproxy/v0/management/...`
- `/cliproxy/management...`

Configure the plaintext local management password via `LOCAL_MANAGEMENT_PASSWORD`
or `MANAGEMENT_PASSWORD` in `.env`. The hashed `remote-management.secret-key`
from `config/cliproxy.yaml` never traverses the proxy.
