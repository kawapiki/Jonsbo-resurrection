# Building a module

Modules are trusted Go packages compiled into the Windows application, not Go
`.so` plugins. Add code only from sources you trust: module code runs with the
controller's permissions. External applications can instead send typed JSON events
to the authenticated loopback API.

Copy `modules/example` to a new package, remove its tests and write tests for your
own behavior. Change the package name, descriptor ID, description and view IDs.
Keep the package portable by importing the standard library and `pkg/module`.
Register your module in the `moduleFactories` map in
`cmd/jonsbo/runtime_windows.go`. For a copy named `customdemo`, import
`"github.com/kawapiki/Jonsbo-resurrection/modules/customdemo"` there and add:

```go
"customdemo": func(options json.RawMessage) (module.Module, error) {
    interval, err := moduleInterval(options)
    if err != nil {
        return nil, err
    }
    return customdemo.New(interval)
},
```

Set its descriptor ID to `customdemo` and enable it in a local configuration:

```json
{"modules":{"customdemo":{"enabled":true,"options":{"interval":"1s"}}}}
```

Run `go run ./cmd/jonsbo serve --config ./local-config.json` with that file.
The constructor should validate configuration; open native resources inside
`Run`, and defer their cleanup. `moduleInterval` is a helper for modules accepting
only an `interval` option; use your own strict decoder for a different schema.

The required interface is:

```go
type Module interface {
    Descriptor() module.Descriptor
    Run(context.Context, module.Publish) error
}
```

`Run` publishes immutable JSON-compatible snapshots, checks cancellation and waits
on a ticker or a context-aware source. Return publication errors. Never spin or
block indefinitely: the runtime cannot forcibly stop trusted Go code. Coordinate
shared state between `Run`, events and rendering with a mutex. Describe synthetic
data explicitly; represent missing sensor metrics as `nil` (JSON `null`), never
invented zero readings. Keep snapshots under `module.MaxStateBytes`.

Optional capabilities are discovered through these interfaces:

```go
type Renderer interface {
    Render(context.Context, string) (image.Image, error)
}
type EventHandler interface {
    HandleEvent(context.Context, module.Event) error
}
```

Declare every logical view's dimensions in the descriptor. Return
`module.ErrUnavailable` before a first snapshot and `module.ErrNotFound` for an
unknown view. Return newly owned images, honor cancellation and leave USB writes,
rotation and resizing to the display layer. Use `module.ErrInvalid` for malformed
payloads and `module.ErrUnsupported` for unsupported event types. Validate exact
payload fields, sizes and allowed values; never interpret event text as code.

`modules/hardware` wraps the existing telemetry collector and dashboard. Its
minimum interval is 500 ms: it primes counters, waits a full interval, then
publishes fresh `telemetry.Snapshot` values. Views are `summary` (640×480), and
`cpu`, `gpu`, `memory` (640×180). Rendering does not collect another sample.
If collection stalls, frames become unavailable after the greater of three
sampling intervals or three seconds since the last completed sample. Frames also
become unavailable when `Run` exits. State endpoints retain the last published
snapshot: clients must inspect lifecycle `status` and `updated_at` to decide
whether readings are current. A successful state request alone does not prove
that sensors are still updating.

`modules/example` publishes explicit demo state (`demo`, `timestamp`, `tick`,
`message`, `progress`) and provides `demo` (640×180). The application uses a
one-second cadence. Its progress bar blends colors and its generated sparkline
moves each tick. Messages appear in state JSON; the intentionally font-free frame
illustrates drawing primitives. A `message` event with `{"text":"Build complete"}`
updates the next tick; text must be nonblank and at most 200 Unicode characters.

## External notifications and agent adapters

Start the controller with the optional example module enabled:

```powershell
go run ./cmd/jonsbo serve --example
```

In a second terminal, from the same repository directory:

```powershell
go run ./examples/event-producer -token-file ./bin/api-token -message "Build complete"
```

`bin/api-token` is the default token path, relative to the controller's working
directory. If configured differently, use the path reported at startup. The
producer defaults to `http://127.0.0.1:8787`; `-url` accepts another HTTP loopback IP
address. `JONSBO_API_TOKEN` is an alternative to `-token-file`; never commit tokens
or print them. The producer has a five-second timeout, refuses redirects, and
does not echo server response bodies or event contents. Its request is:

```http
POST /v1/modules/example/events
Authorization: Bearer <token>
Content-Type: application/json

{"type":"message","payload":{"text":"Build complete"}}
```

Use this boundary for a notification producer or an AI usage adapter backed by an
official API or an explicitly supplied export. Such adapters, account connectors,
video decoding and real historical charts are extension ideas, not implemented
features. Add a dedicated schema/module when new data needs validation or a new
visualization; do not send credentials or executable commands as events.

Run `go test ./modules/example ./examples/event-producer` on any Go platform.
On Windows, also run `go test ./modules/hardware`; those adapter tests use a fake
collector and do not open hardware or write to displays.
