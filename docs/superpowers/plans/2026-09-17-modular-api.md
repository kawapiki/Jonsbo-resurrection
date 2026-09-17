# Modular API Implementation Plan

> For agentic workers: use superpowers:subagent-driven-development for independent bounded tasks and review the combined implementation.

Goal: a contributor-friendly modular Go controller with a local API, hardware module, and runnable extension example.
Architecture: module contract -> lifecycle/state runtime -> API and display workers. Hardware collection and rendering live in the hardware module; USB remains an adapter.
Tech stack: Go standard library, Windows native APIs, existing approved PawnIO dependency.
Spec: docs/superpowers/specs/2026-09-17-modular-api-design.md

## Constraints

- Windows amd64 hardware support; no new Go dependencies or vendor binaries.
- API binds loopback, bearer authentication, bounded payloads and subscriptions.
- All existing hardware measurements and orientation remain available.
- No publication, invented remote repository, account scraping, or fabricated integration data.

## Tasks

- [x] 1. Add public `pkg/module` contract and `internal/engine` runtime. Test duplicate/invalid descriptors, JSON snapshots copied independently, cancellation, failed/panicking module isolation, latest-only subscription, renderer and event dispatch. Engine methods: Register, Start, Close, Snapshot, Subscribe, Frame, Event, ValidateView.
- [x] 2. Implement `modules/hardware` adapter over telemetry/dashboard and `modules/example` runnable event/render demo. Tests use fake hardware collector and deterministic example options. Provide external producer under examples/event-producer using public HTTP contract.
- [x] 3. Implement `internal/api`: Handler backend interface uses pkg/module states/events; GET /v1/modules, /v1/state, /v1/events, /v1/modules/{id}, /v1/modules/{id}/views/{view}.png; POST /v1/modules/{id}/events; GET /v1/displays and PUT /v1/displays/{serial}/assignment. Test authentication, bad methods/JSON/bounds, SSE and PNG, and display routing. API never returns USB device paths.
- [x] 4. Extract generic display delivery from CLI into `internal/output`; integrate runtime with `monitor` and new `serve`. Add token/config helpers and script launch options. Validate loopback-only address, config assignments, graceful shutdown, no preview-only USB ownership, and real JSON/API reads.
- [x] 5. Prepare README, architecture/API/module-author guides, example config, contribution/security/release checklist and Windows CI. Prepare draft MIT license, pending final release decision. Include third-party notices and keep machine-private artifacts excluded.
- [x] 6. Run tests/vet, independent review, actual API/preview/event tests and live screen verification; leave current hardware dashboards functioning. Record outcomes and remaining release decisions.

## Progress and rulings

- Existing branch contains the previous sensor work; retained it in-place; no checkpoint commit was created. Changes remain reviewable in the working tree.
- User directly requested implementation; apply the design/planning structure without adding redundant design permission gates.
- License preference asked asynchronously; other implementation is independent of that decision.

## Verification outcomes

- All Windows package tests passed uncached; go vet and build passed.
- API authentication, bounded JSON, SSE, PNG previews, events, runtime lifecycle, output routing, and stale hardware behavior have automated coverage.
- Live API returned elevated PawnIO CPU temperature and AMD dedicated VRAM used/total. All four displays reported live USB acknowledgements with hardware assignments.
- Temporarily assigned the example animation to one fan through HTTP; live USB acknowledgement succeeded, then restored its hardware view and rotation. Hardware preview and example event also succeeded.
- Local Windows binary bundle and checksum generated from an explicit file list, including licenses and signed-module matching source. No publication or remote CI run performed.
- Independent review found stale hardware render risk, now fixed and re-reviewed without further important findings.
- The modular server remains running with hardware dashboards, API and example module enabled. No new physical orientation confirmation was requested; prior confirmed transforms are preserved.
- MIT is a draft default stated after the unanswered license preference question. Hosting/name, maintainer contact, license confirmation and broader hardware testing remain publication decisions.
