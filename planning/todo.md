# HelixRun TODO

## Backlog

- [ ] [ID:HR-002] [area:cliproxy] Uitbreiden van cliproxy HTTP-routes en handlers voor toekomstige HelixRun-features

## In Progress

## Done

- [x] [ID:HR-001] [area:meta] Basisprojectstructuur opgezet voor HelixRun met embedded CLIProxyAPI en proxy-endpoint `/cliproxy/*`
- [x] [ID:HR-003] [area:store] Postgres-gebaseerde credential store + `/api/credentials` router/handler/repo structuur gerealiseerd met schema in `database/schema.sql`
- [x] [ID:HR-004] [area:ui] Eenvoudige HTML/JS-UI voor credentialbeheer (`/admin/credentials.html`) die `/api/credentials` gebruikt en OAuth/device-flow logins via de embedded CLIProxy management endpoints laat uitvoeren.
