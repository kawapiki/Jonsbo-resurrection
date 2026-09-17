# Windows prototype verification

Date: 2026-09-17. Platform: Windows amd64, Go 1.27.1 (project-local archive SHA-256 checked against official release metadata).

## Hardware results

| Screen | Serial | Final static transaction | Bytes including initialization |
| --- | --- | --- | --- |
| Pump 1 | PUMP_SERIAL | 27 ms, matching opcode/timestamp replies | 28149 |
| Fan 2 | FAN_SERIAL_1 | 352 ms with final input-drain fix, reset59 and frame62 replies | 368704 |
| Fan 3 | FAN_SERIAL_2 | 249 ms, reset59 and frame62 replies | 368704 |
| Fan 4 | FAN_SERIAL_3 | 250 ms, reset59 and frame62 replies | 368704 |

These are single static initialization-plus-frame measurements, **not** sustained frame rates or a benchmark against the vendor application. The fan sequence includes a deliberate 200 ms reset delay. The final review fix adds a 100 ms idle receive boundary, hardware-tested on fan 2; fan 3/4 measurements precede that fix. Process startup is excluded.

The owner confirmed: **“All four look correct.”** This was after correcting fan orientation to landscape. The RGB stripes, white top stripe and distinct numbers were the calibration pattern. Vendor JONSBO was stopped to release the handles; no firmware or cooling settings were changed.

## Tests

The suite covers supported device filtering, explicit unambiguous selection, command fields and encryption envelopes, captured fan command/reply fixtures, frame numbering and boundaries, wrong-size rejection, image row/color conversion, aspect ratio/letterboxing, rotation, cancellation, short writes, missing replies, bounded empty reads and pump reply correlation. A separate review caught stale fan59/62 replies falsely confirming a new frame; its regression failed before the fix and passes with the bounded input drain. Follow-up review found no remaining concrete issue in that fix.

Run `scripts/build.ps1` to repeat tests, vet and build. Run `bin/jonsbo.exe list` for live enumeration. Hardware write tests are explicit CLI commands and are not part of `go test`.

## First-milestone scope

Delivered: serial-addressed CLI, static PNG/JPEG input, generated patterns, optional logical previews, configurable rotation, WinUSB discovery, bounded transactions, handle cleanup, protocol documentation and a Windows executable.

Not yet delivered: background service, reconnect worker, live sensors, animated content, visual layout editor, notification ingestion or AI-agent integrations. The CLI re-enumerates each invocation and has no frame queue. Long-running reliability and hot-unplug behavior still need physical testing when those features are implemented.
