# HelixRun API Endpoints

## `/api/credentials`

### GET

List all stored credentials.

**Headers**
- `Authorization: Bearer <LOCAL_MANAGEMENT_PASSWORD>` or `X-Management-Key: <LOCAL_MANAGEMENT_PASSWORD>`

**Response**
```json
{
  "credentials": [
    {
      "id": "gemini-123.json",
      "provider": "gemini",
      "label": "dev api key",
      "status": "active",
      "disabled": false,
      "attributes": { "path": "auths/gemini-123.json" },
      "metadata": { "type": "gemini" },
      "created_at": "2025-12-10T09:00:00Z",
      "updated_at": "2025-12-10T09:00:00Z"
    }
  ]
}
```

### POST

Create or import a credential.

**Headers**
- `Authorization` or `X-Management-Key` as above
- `Content-Type: application/json`

**Body**
```json
{
  "id": "optional-id.json",
  "provider": "gemini",
  "label": "dev api key",
  "attributes": {
    "api_key": "sk-..."
  },
  "metadata": {
    "type": "gemini",
    "project_id": "project"
  },
  "disabled": false
}
```
If `id` is omitted the server generates `<provider>-<uuid>.json`.

**Responses**
- `201 Created` with credential object on success
- `400 Bad Request` for invalid payload

### `/api/credentials/{id}`

`{id}` is the credential identifier (e.g. `gemini-uuid.json`).

#### GET

Fetch a single credential. Same headers as GET collection.

**Response**
Credential object or `404 Not Found`.

#### DELETE

Remove a credential and disable it in the runtime manager.

**Response**
- `204 No Content` on success
- `404 Not Found` if the credential does not exist

## OAuth / device-flow logins

HelixRun does not implement provider-specific OAuth flows itself. Instead, it delegates to the embedded
CLIProxyAPI management API and uses the shared credential store so that all resulting auth files are
persisted in PostgreSQL and exposed via the `/api/credentials` endpoints above.

The browser UI in `config/static/credentials.html` calls the following management routes via the proxied
`/cliproxy` prefix (the `X-Management-Key` header is injected automatically by HelixRun):

- `GET /cliproxy/v0/management/codex-auth-url?is_webui=1`
- `GET /cliproxy/v0/management/anthropic-auth-url?is_webui=1`
- `GET /cliproxy/v0/management/antigravity-auth-url?is_webui=1`
- `GET /cliproxy/v0/management/gemini-cli-auth-url?is_webui=1`
- `GET /cliproxy/v0/management/qwen-auth-url?is_webui=1`
- `GET /cliproxy/v0/management/get-auth-status?state=...`
- `GET /cliproxy/v0/management/github-copilot/token` (GitHub Copilot, if supported by your CLIProxy build)

Each successful OAuth/device-flow login creates or updates a provider auth file in the configured
`auth-dir`. The custom auth store in this project mirrors those files into the `provider_credentials`
table, so the resulting credentials appear in `GET /api/credentials` without additional endpoints.
