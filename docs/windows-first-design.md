# Windows-first display controller: proposed design

## First deliverable

A Go command-line application that discovers the four connected JONSBO TF3-360SC displays, lists their serial numbers, and sends a static test image to an explicitly selected display. Acceptance requires an observed correct image on each physical panel and correct reporting of failed transfers. Protocol investigation precedes enabling image writes.

## Architecture

- Native Windows SetupAPI discovery and WinUSB transport, reusing the installed WinUSB driver.
- Separate protocol adapters for 1CBE:0035 and 43A8:0E61 until evidence proves shared behavior.
- Stable device selection by serial number. Physical labels (pump, fan-left, fan-middle, fan-right) are assigned after visual identification.
- Image decoding, resizing and encoding separated from USB I/O. Dimensions, orientation and packet format must come from verified vendor behavior.
- One worker per device, bounded write timeouts, explicit close/cancellation and automatic rediscovery after disconnect. No unbounded frame backlog.
- Exclusive ownership during custom display operation; vendor software must release the device before write testing.

## Subsequent milestone

A background Go app with editable layouts and a loopback-only JSON event API. Events carry source, status, text, priority and expiry. Agent adapters submit running, waiting, completed and failed events through that API. Specific agent integrations and Windows notification ingestion are separate adapters, not assumed capabilities of the hardware.

A lightweight local browser interface can configure layouts later; the first milestone needs no GUI framework.

## Alternatives considered

1. Native WinUSB from Go (recommended): uses current drivers and keeps runtime dependencies small, with Windows-specific discovery code.
2. Go plus libusb: portable transport abstraction but adds native library packaging; defer unless native WinUSB proves unsuitable.
3. Wrap the vendor application: might deliver features sooner but retains its runtime and reliability limitations.

## Verification

First recover and cross-check packet structure against captured vendor transfers. Test packet encoders against those reference bytes, exercise transfer failure and disconnect handling, then verify a known static image on all four panels. Record observed update rate and resource use before making performance claims.

## Implementation status

Approved and implemented as a static-image CLI on 2026-09-17. All four screens have been visually verified. See protocol.md and verification.md for evidence. The background worker/reconnection architecture and subsequent event API remain future work; the CLI owns one device per invocation and rediscovers it on each run.
