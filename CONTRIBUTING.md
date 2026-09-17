# Contributing

The supported hardware target is Windows amd64. Go 1.24 or newer is required.
The runtime, API, configuration, abstract display delivery and example module are
portable; the complete application and hardware packages are Windows-specific.

Keep changes focused and describe the behavior being changed, the verification
performed and any hardware limits. For a module, start with [the module guide](docs/modules.md).
Preserve the verified USB framing, acknowledgement handling and retries unless a
change includes evidence for the affected device. Missing readings must remain
unavailable (`null` in JSON), not invented zero values.

## Local verification

From the repository root, in an ordinary Windows terminal:

```powershell
go test ./...
go vet ./...
go build -o ./bin/jonsbo.exe ./cmd/jonsbo
```

Default automated tests use fakes or controlled local resources. They must not connect to
physical displays, install drivers, require elevation, stop a running controller
or send USB frames. Add meaningful tests for lifecycle, cancellation, validation,
protocol parsing and failure handling. Module tests should inject a collector or
driver rather than depend on readings from the contributor's machine. Existing
native AMD probe tests are explicitly opt-in; leave their live-test environment
flags unset for ordinary checks and CI.

For portable packages on Linux or macOS:

```sh
go test ./pkg/module ./internal/engine ./internal/api ./internal/config ./internal/output ./modules/example ./examples/event-producer
go vet ./pkg/module ./internal/engine ./internal/api ./internal/config ./internal/output ./modules/example ./examples/event-producer
```

Full `go test ./...` on Linux is unsupported because it includes Windows hardware
code. CI runs the complete checks and build on Windows amd64 with Go 1.24.x and
the current stable Go version; a separate Ubuntu job checks only portable packages.

## Manual checks

Manual hardware testing is opt-in and requires the device owner's approval.
Agree which controller owns each display before starting or stopping a process.
Record the device model, tested behavior, commands and results without publishing
serial numbers, personal paths or captures containing private data.

`go run ./cmd/jonsbo serve --example` provides API and PNG previews without USB
display ownership. It still samples host sensors. See [the module guide](docs/modules.md)
for the external event producer. Use `--all` only during an agreed hardware test.
Do not run development commands as administrator by default. PawnIO temperature
access needs approved driver setup and elevation; obtain explicit approval for
that manual test, and keep it out of automated tests and CI.

## Repository hygiene

Never commit API tokens, AI account credentials, logs containing private data,
machine-specific configuration, downloaded vendor DLLs, binary captures or local
developer tools. Review untracked files before staging. Keep third-party source
and license notices intact. CI builds and checks code; it does not publish,
deploy or create releases.

The project license remains a maintainer decision. Do not add a license grant or
relicense existing code without that decision. Contributions containing third-party
material must identify its origin and applicable license.
