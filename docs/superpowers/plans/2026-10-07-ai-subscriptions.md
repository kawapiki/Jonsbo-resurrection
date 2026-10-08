# AI subscriptions implementation plan

> **For agentic workers:** Use superpowers:subagent-driven-development. The user selected parallel specialized agents; disjoint file ownership overrides the skill's sequential-implementer default.

**Goal:** Connect native subscription accounts and display trustworthy usage/activity through assignable AI widgets and an optional browser companion.

**Architecture:** A portable Go module owns normalized observations, aggregates, persistence and rendering. A provider manager owns native OAuth helpers and metadata parsers. The existing authenticated loopback API and configurator compose them; a native messaging host relays browser metadata using a separate scoped credential.

**Tech stack:** Go standard library, embedded vanilla JavaScript/CSS, Chromium Manifest V3, Windows process helpers. No API-key billing or new external Go dependency.

**Spec:** [Approved design](../specs/2026-10-07-ai-subscriptions-design.md).

## Global constraints

- Preserve loopback Host/Origin checks and bearer authentication. Browser scripts never receive master credentials.
- Unknown metrics remain null; quotas, consumed tokens and current context are separate.
- Account observations require verified identity. Unattributed sessions remain local observations.
- No inference requests, private endpoints, credential scraping, transcript parsing or chat-content storage.
- One-second publication/render cadence, 60-second account polls, 180-second account freshness, 45-second browser freshness.
- Bound history to 35 days, sessions to 128, deduplication to 200,000 keys, storage to 32 MiB.
- Native login and local helper windows are hidden; interactive OAuth completes in the system browser.
- Existing layouts, sensors and USB protocols remain compatible.

## Review focus

- Replay after restart must not inflate usage; tests persist/reload a request ID and deliver it twice.
- Account switches must not merge records; tests send the same session ID under two accounts.
- Compaction and absent data must not invent context or weekly tokens; tests clear context and assert null values.
- Browser suspension and spoofed origins must not remain live; tests expire heartbeat and reject invalid origins/ingress credentials.
- Native process cancellation and protocol failure must release resources; adapter tests use controlled readers/process fakes.

## Shared contracts

`pkg/aisubscriptions/model.go` is root-owned, contains JSON records only, and is frozen before parallel work. `Observation` is the sole normalized intake. Adapters use `type Sink func(context.Context, Observation) error`. Module state contains `Providers` and `Sessions` arrays. Root owns command/API wiring and browser host; workers must not edit those files.

## Task 1: Normalized module, persistence and rendering (core specialist)

**Own:** `modules/aisubscriptions/**`. Read `pkg/aisubscriptions/model.go`, `pkg/module`, approved spec and research.

**Produce:**

```go
func New(dir string) (*Module, error)
func (m *Module) Observe(context.Context, data.Observation) error
func (m *Module) Snapshot() data.State
func DrawWidget(data.State, string, int, int, time.Time, bool) image.Image
```

Module implements Module/Renderer/EventHandler; event type `observation`; declared views `overview`, `openai`, `claude`, `sessions`. `DrawWidget` supports portable configurator rendering and uses local official provider images when present.

- [x] Write and run failing tests for duplicate request/restart, isolated accounts, unknown fields, compaction, DST/week interval, stale observations and bounds.
- [x] Implement strict observation validation, aggregation and atomic versioned persistence. Inputs over limits fail without mutating state.
- [x] Implement native-sized views and connected/activity animation; return fresh owned images and honor cancellation.
- [x] Run `go test ./modules/aisubscriptions` and write the task report.

Example acceptance assertion: after applying `usage(request_id="r1", tokens=50)` twice and reopening storage, observed weekly consumption remains 50, never 100.

## Task 2: Native account adapters and telemetry (provider specialist)

**Own:** `internal/aiadapters/**`, `cmd/jonsbo-ai-observer/**`. Read shared contract, spec/research and ignored installed Codex schema.

**Produce:**

```go
type Sink func(context.Context, data.Observation) error
func New(dir string, sink Sink) (*Manager, error)
func (m *Manager) Run(context.Context) error
func (m *Manager) Handler() http.Handler
func (m *Manager) AuthenticateIngress(*http.Request) bool
```

Controller routes are `/v1/ai/status`, `/v1/ai/providers/{openai|claude}/connect`, `/disconnect`, `/refresh`, `/v1/ai/setup`, `/v1/ai/telemetry/{openai|claude}/v1/logs`, and scoped `/v1/ai/browser/events`. Browser credential is `browser.token` in manager data directory. `AuthenticateIngress` accepts only POST browser events with that credential. Master-auth controller setup may generate browser/hook scoped credentials, but never exposes provider tokens.

Status JSON includes provider readiness/connection and setup instructions; connect result may contain transient `auth_url`, `verification_url`, `user_code`, never OAuth tokens. OpenAI child uses its dedicated native home. Claude connection uses native subscription login, and disconnect only detaches. User setup must preserve existing hooks/status-line/exporters.

Observer helper command supports `hook` and `statusline`, stdin JSON, loopback API destination, scoped observer credential file, bounded requests and silent hook output (`{}` for Codex Stop where required). Statusline prints a useful native line after forwarding selected fields. Native telemetry HTTP intake projects only metadata/request token fields. Use request IDs, not process sequence alone, for consumption keys.

- [x] Write failing parser/auth/JSON-RPC/cancellation tests with synthetic fixtures and fake native command runners.
- [x] Implement subscription-mode verification, OAuth lifecycle, bounded native processes and 60-second polls with backoff.
- [x] Implement status-line/hook/OTLP JSON projection and browser ingress sanitization. Never store/log full source payloads.
- [x] Implement reviewable setup snippets and precise prerequisite/version errors; do not mutate native user config automatically.
- [x] Run `go test ./internal/aiadapters ./cmd/jonsbo-ai-observer` and write report.

## Task 3: Configurator AI widgets and connection UI (interface specialist)

**Own:** `modules/configurator/**`, `internal/configuratorweb/**`. Root/other workers will not edit these files while task runs.

Consume module state by decoding `ai-subscriptions` snapshots using shared `data.State`; use core `DrawWidget` for server canvas rendering. Add optional overlay fields `provider`, `animate` (pointer bool, default true), and `detail`. `ai-provider` remains valid with existing layout fields, supports `openai` and `claude`, and uses server state freshness. Fixed numeric AI metric IDs include `ai.openai.weekly`, `ai.openai.tokens`, `ai.openai.sessions`, and Claude equivalents. Weekly metric is percent; tokens are observed/reported weekly tokens; absent values null.

Connect controls use root-mounted `/v1/ai` controller. Preserve current editor visual language with visible provider marks and focused animation. Add browser/server-consistent widget settings, reduced motion, existing Save & apply flow and missing-data guidance.

- [x] Add failing Go validation/sampling/render tests and JavaScript tests for AI widget defaults, null metrics and settings.
- [x] Implement AI overlay/catalog, state extraction and renderer; do not apply the hardware three-second freshness threshold to account data.
- [x] Implement provider connection/setup UI and draggable AI widgets; a transient auth URL is opened only from an explicit connect action.
- [x] Verify backward-compatible saved layouts and run both package and JavaScript tests; write report.

## Task 4: Composition, browser companion, documentation and verification (root)

**Own:** `pkg/aisubscriptions/**`, `cmd/jonsbo/**`, `internal/api/**`, `internal/config/**`, `cmd/jonsbo-ai-bridge/**`, `integrations/browser/**`, `scripts/**`, `docs/**`, `README.md`, `configs/**`.

- [x] Create shared JSON contract and save ownership/decisions ledger before parallel dispatch.
- [x] Add module registration/data path configuration, managed adapter lifecycle, opt-in enablement and authenticated controller dispatch. Add observer scoped ingress without altering Host/Origin guards.
- [x] Implement bounded Chrome native-message framing and local HTTP relay; authenticate with browser-only credential. Validate allowed provider URLs and observation type at both boundaries.
- [x] Build optional MV3 extension: visible DOM metadata only, popup pairing/status, per-tab 15-second heartbeat, removal on close, no cookies/network/chat bodies. Provide local Windows native-host registration script; installing extension/native host is a user action.
- [x] Write bridge/extension/API integration tests with synthetic metadata. Check that scoped browser credentials cannot access controller/account routes.
- [x] Review each worker's report and diff; dispatch independent security/quality review when slots become available. Fix findings and integrate.
- [x] Run full Go tests/vet, JavaScript tests, build Windows CLI/GUI/helpers, exercise authenticated API/preview with no USB writes and inspect both physical aspect ratios.
- [x] Update module/setup/security/docs and report remaining live OAuth/browser setup requirements accurately.

## Execution rulings

User approval of the written design and explicit “create ... with parallel specialized agents” authorizes implementation; no extra plan-permission round is added. Reuse the existing non-main task checkout. Parallel workers edit disjoint owned files and do not commit shared state. Root handles integration and final independent review. Credentials are only generated within local ignored data paths; no real account sign-in is attempted without a user-triggered product action.
