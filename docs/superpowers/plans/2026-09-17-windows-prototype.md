# Windows display prototype Implementation Plan

> Execute inline using the executing-plans workflow, with tests for protocol and transfer failure behavior.

**Goal:** Discover and address the four JONSBO screens from a Windows Go CLI and verify static image transfers.
**Architecture:** Windows SetupAPI discovers installed WinUSB interfaces; a small transport owns each handle. Separate display protocol encoders produce packets independently of I/O. The CLI selects by serial number and never guesses a target.
**Tech Stack:** Go standard library, Windows SetupAPI and WinUSB, local protocol fixtures.
**Spec:** docs/windows-first-design.md (approved by user).

## Global constraints

- Windows first, existing WinUSB drivers.
- Two hardware IDs: 1CBE:0035 and 43A8:0E61.
- Hardware writes follow verified protocol evidence.
- Vendor binaries, captures and development toolchains remain ignored local research artifacts.
- No GUI or event API in this first milestone.

## Task 1: Recover protocol evidence

- [x] Obtain a project-local Go toolchain and static .NET decompiler.
- [x] Diagnose capture failure from process output and upstream source; verify live USB endpoints.
- [x] Extract relevant vendor packet builders and image-transfer paths without running vendor methods.
- [x] Document commands, payload format, dimensions, endpoints and sequence with evidence provenance in docs/protocol.md.

## Task 2: Device discovery and WinUSB transport

Files: internal/device/device.go, internal/device/device_test.go, internal/winusb/winusb_windows.go, cmd/jonsbo/main.go.

- [x] Write selection tests: empty serial, unknown serial and duplicate matches must fail; known serial resolves exactly one supported device.
- [x] Run tests and observe missing implementation.
- [x] Implement SetupAPI interface discovery, serial-based filtering and CLI list output.
- [x] Query interface endpoints without display writes; verify all four IDs against live enumeration.
- [x] Add handle ownership, timeout policy and short-transfer/error reporting.

## Task 3: Protocol encoding and image command

Files: internal/protocol/*, internal/display/*, internal/imageutil/*, corresponding *_test.go files.

- [x] Write packet fixture tests using independently recovered vendor bytes. Reject oversized data and invalid image geometry.
- [x] Observe failures, implement only evidence-backed encoders, rerun tests.
- [x] Add real image decoding and rendering with deterministic PNG test patterns.
- [x] Add transfer sequencing tests for errors, short writes, cancellation and bounded updates.
- [x] Add CLI image command requiring explicit serial selection and reporting errors.

## Task 4: Hardware verification and documentation

Files: README.md, docs/protocol.md, docs/verification.md.

- [x] Run go test ./..., go vet ./... and build bin/jonsbo.exe.
- [x] Ensure vendor software releases displays before custom writes.
- [x] Send a known static image to each physical screen; record successful writes separately from visual confirmation.
- [x] Verify cleanup and rediscovery, and document actual supported behavior and remaining gaps.

Implementation note: static-image CLI is complete and visually confirmed. A long-running worker/reconnection loop belongs to the subsequent background-app milestone; this CLI re-enumerates on each invocation. Protocol acknowledgement and stale-queue issues found during hardware tests/review are fixed and covered by regression tests.

