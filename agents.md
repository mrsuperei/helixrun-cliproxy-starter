# HelixRun Agent Contract (Codex / Code Agents)

Dit document beschrijft hoe generieke code-agents (zoals Codex of vergelijkbare LLM-code agents) zich binnen de HelixRun-repository moeten gedragen.

Doelen:

- HelixRun en `CLIProxyAPI-Extended` (en de onderliggende `CLIProxyAPI`-architectuur) correct begrijpen en respecteren
- Idiomatische, veilige en onderhoudbare Go-code genereren
- Taken en documentatie consequent bijwerken in `/planning` en relevante `README.md`-bestanden
- Geen project-specifieke logica hard-coderen als dit via configuratie (bijv. `config.yaml` of database) kan

Gebruik dit bestand als primair contract voor elke agent die code of configuratie in deze repo genereert of wijzigt.

---

## 1. Rol van de agent binnen HelixRun

1. De agent is een **Go-georiënteerde systeem- en code-assistent** die:

   - HelixRun integreert met `CLIProxyAPI-Extended` als universele LLM-router / proxy
   - Providers, modellen en routing inricht via `config.yaml`, `auth-dir` en eventueel de Go SDK
   - User prompts omzet naar concrete taken in `/planning` (zie sectie 6)
   - De relevante documentatie (`README.md`-bestanden) bijwerkt of aanmaakt

2. De agent MOET:

   - Go 1.21+ gebruiken en idiomatische Go-regels volgen
   - Bestaande projectarchitectuur respecteren (structuur, patterns, naming)
   - `context.Context` consistent gebruiken en propagateren
   - Concurrency veilig toepassen (geen goroutine leaks, duidelijke lifecycle)
   - Configuraties zo veel mogelijk **data-gedreven** houden (`config.yaml`, DB, JSON/YAML), niet hard-coded
   - Bij wijzigingen aan CLIProxy-integratie verwijzen naar de officiële CLIProxyAPI/CLIProxyAPI-Extended guides

3. De agent MAG:

   - Kleine refactors uitvoeren om code beter testbaar of uitbreidbaar te maken
   - Nieuwe `README.md`-bestanden aanmaken als die nodig zijn om gebruik / inrichting uit te leggen
   - Taken opsplitsen of herformuleren zolang de planning in `/planning` consistent blijft
   - De HelixRun-binary laten fungeren als “embeddende host” voor CLIProxyAPI via de Go SDK

4. De agent MAG NIET:

   - Eigen ad-hoc proxy’s of provider-clients bouwen die de bestaande CLIProxyAPI(-Extended) functionaliteit dupliceren
   - Grote, risicovolle rewrites doen zonder duidelijke reden en zonder dit in planning + docs te beschrijven
   - Willekeurig nieuwe top-level directories toevoegen buiten de bestaande architectuur
   - Configuratie van providers/models verspreid hard-coden in de codebase; dit hoort centraal in CLIProxy’s `config.yaml` / management API / HelixRun-config

---

## 2. Architectuur: HelixRun en CLIProxyAPI-Extended

HelixRun gebruikt **CLIProxyAPI-Extended** als centrale LLM-router. De belangrijkste principes:

- **CLIProxyAPI-Extended** is een fork van `router-for-me/CLIProxyAPI` met:
  - Een canonische IR-vertaler (hub-and-spoke) voor alle providers
  - Extra providers (zoals Kiro, GitHub Copilot, Cline, Ollama)
  - Compatibele OpenAI/Gemini/Claude/Codex-API endpoints

- **HelixRun** ziet CLIProxyAPI-Extended als een generieke “LLM-gateway”:
  - HTTP-interface: OpenAI-achtige endpoints (`/v1/chat/completions`, etc.)
  - Embedded Go-service: via `sdk/cliproxy` in-process te draaien
  - Management API: runtime beheer via `/v0/management/...` waar relevant

Belangrijke componenten aan CLIProxy-zijde:

- **HTTP server & request pipeline**  
  Verantwoordelijk voor:
  - Luisteren op de geconfigureerde poort
  - Routing naar OpenAI-/Gemini-/Claude-/Codex-compatibele endpoints
  - Middleware (logging, CORS, auth, management)

- **Configuratie & storage**
  - `config.yaml` voor providers, modellen, routing, logging, ports, management settings
  - `auth-dir` met OAuth-/API-sleutels (per provider)
  - Optionele storage-backends (Git, PostgreSQL, object storage) voor state/config

- **Providers & executors**
  - Eén uniforme ProviderExecutor-interface voor alle upstream providers
  - OpenAI-compatible providers, Gemini CLI, Claude Code, Qwen, iFlow, Antigravity, etc.
  - In CLIProxyAPI-Extended: extra providers en een Canonical IR-translator-laag

- **Go SDK (`sdk/cliproxy`)**
  - Embed de volledige proxy-server in een Go-applicatie (zoals HelixRun)
  - Regelt lifecycle (start/stop), config-watching, auth-management, health & logging

De agent moet:

- HelixRun-code zo schrijven dat **HelixRun alleen met CLIProxyAPI praat**, niet rechtstreeks met willekeurige LLM-API’s (tenzij expliciet anders gevraagd)
- Provider-specifieke logica (auth, modelnamen, compat-mode, enz.) zoveel mogelijk overlaten aan CLIProxyAPI(-Extended)
- HelixRun-config zó vormgeven dat het makkelijk is om:
  - Tussen verschillende CLIProxy-instances te wisselen (dev/stage/prod)
  - Nieuwe providers of modellen toe te voegen via configuratie i.p.v. code

---

## 3. Projectstructuur die de agent moet volgen

Tenzij de repo expliciet anders is ingericht, hanteert HelixRun de volgende conventie:

- `/cmd/server`  
  Entrypoint voor de server (HTTP/gRPC), wiring van integratie met CLIProxyAPI.

  - `/internal/store`  
  Database connectie/config ect

- `/internal/cliproxy`  
  alles gerelateerd aan cliproxy. zorg dat alle endpoints hier ook onder routes worden opgeslagen

- `/pkg/utils`  
  Algemeen herbruikbare helpers, geen domeinspecifieke logica.

- `/planning`  
  Planning- en ontwerpdocumenten, waaronder planning van taken en roadmap.

De agent moet nieuwe code onder een passend `internal`-pakket plaatsen en consistent zijn met de bestaande layout.

---

## 4. Standaard Go-richtlijnen

De agent moet standaard Go-richtlijnen respecteren, zowel qua stijl als architectuur.

### 4.1 Taalversie en modules

- Gebruik **Go 1.21+**
- `go.mod` moet geldig zijn en module path consistent met de repo-root.
- Nieuwe externe dependencies beperken en goed motiveren in comments/README.

### 4.2 Naamgeving en structuur

- Pakketnamen kort en beschrijvend (`graph`, `agents`, `store`, `telemetry`, `http`).
- Functies en types met duidelijke namen; exported types en functies beginnen met een hoofdletter.
- Vermijd “god types” of te grote bestanden; splits logische componenten op.

### 4.3 Errors en logging

- Retourneer errors expliciet: `(..., error)` of alleen `error`.
- Wrap errors waar context belangrijk is:

  ```go
  if err != nil {
      return nil, fmt.Errorf("failed to load session %s: %w", sessionID, err)
  }
  ```

- Logging niet in library-functies hard-coderen; gebruik bij voorkeur een geïnjecteerde logger of telemetrystelsel.
- Geen panics gebruiken voor normale foutafhandeling.

### 4.4 Context

- Elke functie die I/O doet,  met DB/Redis werkt of langdurig draait, moet `ctx context.Context` accepteren.
- Context wordt doorgegeven naar onderliggende calls en mag niet genegeerd worden.
- Bij blocking loops of worker-goroutines moet `ctx.Done()` worden gecontroleerd:

  ```go
  select {
  case <-ctx.Done():
      return ctx.Err()
  case msg := <-ch:
      // verwerk msg
  }
  ```

### 4.5 Concurrency en goroutines

- Goroutines alleen starten als dat nodig is, en altijd met een duidelijke exit-conditie (bijv. context cancel).
- Geen onbeperkte goroutine-creatie in request-handlers.
- Gebruik evt. worker pools of bounded channels als er veel parallel werk is.

---

## 5. Integratie met CLIProxyAPI-Extended

CLIproxy api draait altijd lokaal, en alle endpoints moeten altijd worden geproxy'd naar /cliproxy. waarbij dan standaard de localmanagement password doorgegeven, zodat die niet via de proxy meegestuurd moet worden.

### 5.1 Deploymentmodellen


1. **Embedded Go-service (SDK):**
   - HelixRun embed de proxy via `sdk/cliproxy` in dezelfde binary.
   - Config en auth blijven in `config.yaml` + `auth-dir`, maar lifecycle (start/stop) wordt door HelixRun beheerd.
   - Management API kan optioneel worden geactiveerd via `config.yaml` (bijv. `remote-management.secret-key`).



### 5.2 HTTP-integratie (OpenAI-/Gemini-compatibel)

De agent gaat ervan uit dat CLIProxyAPI(-Extended):

- OpenAI-compatible endpoints aanbiedt (zoals `/v1/chat/completions`, `/v1/completions`, `/v1/models`)
- Streaming (`stream: true`) en non-streaming responses ondersteunt
- Tool/function calling ondersteunt volgens het OpenAI chat-format

Regels:

1. Maak in HelixRun **geen** directe provider-clients aan (OpenAI, Gemini, Claude, …) als dat via CLIProxyAPI kan.
2. Configureer een generieke OpenAI-compatible client tegen het CLIProxy-endpoint:
   - Base URL: bijv. `http://localhost:8317/v1`
   - API-key: dummy of CLIProxy-specific header als dat nodig is (volgens config / docs)
3. Zorg dat modelnamen configureerbaar zijn via HelixRun-config, zodat `provider:model` combinaties makkelijk zijn aan te passen.

### 5.3 Embedding via de Go SDK (`sdk/cliproxy`)

Voor in-process integratie gebruikt de agent de officiële SDK zodat HelixRun zelf de CLIProxy-server start.

#### 5.3.1 Imports (kernpakketten)

Een minimale embed gebruikt ten minste:

```go
import (
    "context"
    "errors"

    "github.com/router-for-me/CLIProxyAPI/v6/internal/config"
    "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy"
)
```

De agent moet de bestaande imports in de HelixRun-repo volgen als daar al een conventie bestaat.

#### 5.3.2 Minimal embed

Een minimale embed binnen HelixRun:

```go
cfg, err := config.LoadConfig("config.yaml")
if err != nil {
    return fmt.Errorf("load cliproxy config: %w", err)
}

svc, err := cliproxy.NewBuilder().
    WithConfig(cfg).
    WithConfigPath("config.yaml"). // absolute of working-dir relatief
    Build()
if err != nil {
    return fmt.Errorf("build cliproxy service: %w", err)
}

ctx, cancel := context.WithCancel(ctx) // afgeleid van HelixRun root context
defer cancel()

go func() {
    if err := svc.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
        // log error via HelixRun-telemetry
    }
}()
```

Vereisten:

- Lifecycle van `svc.Run` moet gekoppeld zijn aan de HelixRun server-lifecycle.
- Shutdown gebeurt door het cancelen van de parent context of een expliciete `svc.Shutdown(ctx)`.

#### 5.3.3 Server-opties & hooks

Bij gevorderde integratie kan de agent, indien nodig:

- Extra middleware toevoegen (bijv. HelixRun-tracingheaders)
- Extra routes registreren (zoals health-checks)
- Request-logging integreren met HelixRun-logging

Dit gebeurt via `cliproxy.WithServerOptions(...)` en `cliproxy.Hooks` in de builder. De agent volgt hierbij de patronen uit de officiële SDK-docs, in plaats van eigen wrappers te bouwen.

### 5.4 CLIProxyAPI-Extended-specifieke instellingen

CLIProxyAPI-Extended introduceert extra configuratie in `config.yaml`, zoals:

```yaml
use-canonical-translator: true   # schakelt de nieuwe IR-architectuur in
show-provider-prefixes: true     # optioneel, providerprefixen tonen in modellijsten
```

Regels:

1. De agent mag deze instellingen **aanzetten** waar dat zinvol is, maar moet altijd:
   - Wijzigingen documenteren in een relevante `README.md`
   - Een taak opnemen in `/planning/todo.md` als dit impact heeft op gedrag, compatibiliteit of debugging

2. Nieuwe providers (bijv. Ollama, Kiro, GitHub Copilot, Cline) worden bij voorkeur geconfigureerd:
   - Via CLIProxy’s provider-configuratie en model-mapping
   - Niet via hard-coded lijsten in HelixRun

---

## 6. Planning & tasks in `/planning`

Planning en taakbeheer horen **niet** in dit `agents.md`-bestand, maar in een aparte directory:

- Directory: `/planning`
- Aanbevolen bestanden:
  - `/planning/todo.md`
  - `/planning/roadmap.md` (optioneel)
  - `/planning/design-*.md` (optioneel, voor deelontwerpen)

### 6.1 `/planning/todo.md` structuur

De agent MOET alle nieuwe taken en wijzigingen aan taken registreren in `/planning/todo.md`.

Aanbevolen vorm:

```markdown
# HelixRun TODO

## Backlog

- [ ] [ID:HR-001] [area:agents] Korte imperatieve beschrijving
  - Optionele detailregel 1
  - Optionele detailregel 2

## In Progress

- [ ] [ID:HR-010] [area:graph] Taak die momenteel actief is

## Done

- [x] [ID:HR-000] [area:meta] Voorbeeld van een afgeronde taak
```

Regels voor de agent:

1. Elke taakregel gebruikt checklist-syntax `- [ ]` of `- [x]`.
2. Elke taak heeft een ID `[ID:HR-xxx]` en een gebiedstag `[area:...]`.
3. De agent voegt nieuwe taken toe in “Backlog” (tenzij expliciet anders gevraagd).
4. Na afronden van een taak verplaatst de agent deze naar “Done” en zet `- [x]`.

### 6.2 Andere planning-bestanden

- `/planning/roadmap.md` kan gebruikt worden voor langere-termijn doelen.
- `/planning/design-*.md` kan gebruikt worden voor uitgebreide ontwerpen of ADR-achtige notities (Architectural Decision Records).

De agent mag nieuwe files in `/planning` aanmaken als dat helpt om de intentie van de wijzigingen duidelijk te documenteren.

---



### 7.3 Inline comments en doc comments

- `//` commentaar alleen gebruiken als het extra context toevoegt.
- Voor exported functies/types: `// Name ...` doc comments in Go-stijl.
- Geen commentaar dat alleen herhaalt wat de code al duidelijk maakt.

---

---

## 9. Referentie: CLIProxyAPI(-Extended) documentatie

De agent moet de officiële documentatie van `CLIProxyAPI` en de README van `CLIProxyAPI-Extended` als primaire referentie gebruiken voor integraties, configuratie en SDK-gebruik.

Aanbevolen startpunten:

- Overzicht & Quick Start  
  - Quick Start: https://help.router-for.me/introduction/quick-start.html  
  - Introductie & configuratie: https://help.router-for.me/

- Hoofdrepo + SDK-docs  
  - Repo: https://github.com/router-for-me/CLIProxyAPI  
  - Go module: `github.com/router-for-me/CLIProxyAPI/v6`  
  - SDK docs (Go): zie `docs/sdk-usage.md`, `docs/sdk-advanced.md`, `docs/sdk-access.md`, `docs/sdk-watcher.md` in de repo

- CLIProxyAPI-Extended  
  - Fork: https://github.com/HALDRO/CLIProxyAPI-Extended  
  - Extra providers & Canonical IR-configuratie: zie `README.md` in de fork

De agent mag patronen uit de officiële voorbeelden hergebruiken, maar moet altijd controleren of deze passen bij de bestaande HelixRun-structuur en -conventies, en wijzigingen duidelijk vastleggen in `/planning` en relevante `README.md`-bestanden.
