# Modular application and local API

## Scope

Refactor the existing Windows Go controller into an extensible application. Deliver a hardware module, a runnable example module, an authenticated localhost API, configurable display assignments, and contributor/release documentation. Existing verified pump/fan protocols and sensor readings remain intact. Future Claude/Codex adapters, notifications, video decoders, charts, and temperature-driven animation use these interfaces; this milestone does not claim those integrations are implemented.

## Decisions

Use compiled-in Go modules and an external JSON event API. Go shared-object plugins are not suitable for this Windows-first application; subprocess RPC is unnecessary for the initial trusted modules. A module publishes JSON state and optionally implements event handling and image rendering. The runtime owns lifecycle/status and bounded latest-state subscriptions. Renderers return logical images; the display layer alone owns resizing, rotation, USB acknowledgements, and retries.

Use a loopback-only HTTP server with a generated bearer token, no permissive CORS, bounded bodies/concurrency, request timeouts, and a versioned /v1 API. Expose module discovery/state, server-sent full-state snapshots, PNG previews, typed module events, and display assignments/status. No arbitrary shell, dynamic code loading, filesystem paths, credentials, or remote URLs are accepted through the API. Event payloads and token files must not enter logs or source control.

Keep hardware as the first built-in module: current CPU/RAM/AMD/VRAM/PawnIO collector and four dashboard views. Example module demonstrates published data, incoming message events, and a generated animated frame; its synthetic values are explicitly demonstration data. Provide an external Go event-producer example for independent notification/agent adapters. No undocumented account/session scraping for AI usage.

## Public Go contract

`pkg/module`: Descriptor(ID,Name,Version,Description,Views); View(ID,Width,Height); Module {Descriptor() Descriptor; Run(context.Context, Publish) error}; Publish func(any) error; Renderer {Render(context.Context,string)(image.Image,error)}; EventHandler {HandleEvent(context.Context,Event) error}; Event {Type string; Payload json.RawMessage}. State includes descriptor, lifecycle status, last data update time, raw JSON data, and error. Extensions are trusted compiled-in code and must honor cancellation; Go cannot forcibly stop a stalled module.

## Platform and release

Windows amd64 remains the supported hardware platform. Pure runtime/API/example packages remain portable; Windows CI tests the complete app. Preserve standalone stats/image/pattern/monitor commands and add serve. The monitor command must use the same runtime/hardware module rather than duplicate the sampling loop. New serve supports preview/API without owning USB, or --all to drive the attached screens. Existing running dashboards remain in service during development.

License choice is pending a user preference. Do not invent a GitHub owner/repository, publish, tag, or claim production readiness. Keep the third-party PawnIO license/source intact; do not include downloaded vendor DLLs, captures, machine paths/serials in public-facing quickstarts, tokens, or developer tools in release archives.

## Verification

Test module validation/lifecycle, bounded subscriptions, failed module isolation, frame/event routing, cancellation, hardware adapter behavior using a fake collector, API authentication/body limits/event validation/SSE disconnects, display assignment validation, and the example producer. Run all Windows tests and vet. Exercise HTTP with the actual hardware module, preview images, and example events without USB conflicts, then test/restart the modular hardware dashboards on the connected displays. Document unimplemented integrations and hardware/driver limits honestly.
