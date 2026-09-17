# JONSBO display controller for Windows

A Go command-line prototype controlling the four screens of a JONSBO TF3-360SC through the existing Windows WinUSB driver. No vendor application, custom driver, cgo, or third-party Go modules are required at runtime.

**Hardware tested:** all four screens independently displayed numbered RGB patterns on 17 September 2026, with correct orientation confirmed by the owner. This is the static-image milestone; a background service, live sensors, layout editor and notification/agent API are not implemented yet.

## Run

Close JONSBO completely, including its tray process, before running this controller. JONSBO holding the USB handles causes Windows to return `Access is denied`, even for an administrator. Normal display operation has been verified without administrator rights once it is closed.

From the project directory in PowerShell:

```powershell
.\bin\jonsbo.exe list
.\bin\jonsbo.exe inspect --serial 0628C908F4CA0504
.\bin\jonsbo.exe pattern --serial 0628C908F4CA0504 --number 1
.\bin\jonsbo.exe pattern --serial BCD32929C0 --number 2
.\bin\jonsbo.exe image --serial BCD32929C0 --file C:\Images\status.png
```

Use the serial numbers returned by `list` on a different unit. Every image operation requires an exact serial; it never selects an arbitrary USB device.

| Screen label | Serial on this computer | Logical canvas | Native transfer canvas | Default clockwise rotation |
| --- | --- | --- | --- | --- |
| 1 — pump | 0628C908F4CA0504 | 640 × 480 | 640 × 480 | 0° |
| 2 — fan | BCD32929C0 | 640 × 180 | 180 × 640 | 90° |
| 3 — fan | BCD32AE5C2 | 640 × 180 | 180 × 640 | 90° |
| 4 — fan | BCD336DBCE | 640 × 180 | 180 × 640 | 90° |

Fan positions left/middle/right have not been assigned; the numbers identify the physical screens. The landscape default matches this installation. Override it with `--rotate 0`, `90`, `180` or `270` for other mounting orientations.

Images may be PNG or JPEG. They are resized to fit, with black letterboxing, using nearest-neighbor sampling. Alpha is composited on black. Source images are limited to 40 megapixels. Use an image at the logical canvas resolution for exact pixels.

`--preview preview.png` writes the logical image before panel rotation. Example:

```powershell
.\bin\jonsbo.exe image --serial BCD32AE5C2 --file C:\Images\agent-status.png --preview preview.png
```

Successful commands print JSON with transfer duration, bytes written and raw device replies. A hardware acknowledgement proves the transaction completed; it does not by itself prove what a person sees. Errors exit nonzero. Ctrl+C cancels between USB operations; a pending transfer has a two-second timeout.

## Build and test

Windows with Go 1.24 or newer:

```powershell
.\scripts\build.ps1
```

The script runs `go test ./...`, `go vet ./...`, then builds `bin/jonsbo.exe`. It uses the project-local verified Go toolchain if present, otherwise `go` from PATH. No module downloads are needed.

## How it works

- `internal/device`: supported IDs and explicit serial selection.
- `internal/winusb`: SetupAPI enumeration, handle ownership and bounded USB operations.
- `internal/protocol`: separate pump and fan packet encoders.
- `internal/imageutil`: resizing, rotation, BGR conversion and test patterns.
- `internal/display`: ordered transactions, stale reply handling, acknowledgement checks and cancellation.

The pump receives a mode command, a transparent PNG layer clear and a JPEG image. Its replies echo the opcode and timestamp, with status `C8` on observed successes. The fan displays receive raw BGR24 pixels in 720 numbered records and acknowledge the completed frame with `62`.

The application changes display content only. It contains no firmware flashing or cooling-control operations. Handles close when the command exits, and each invocation rediscovers connected devices. There is no resident process or automatic reconnect loop yet. JONSBO can be started again to restore its themes.

## Research and evidence

See [protocol notes](docs/protocol.md), [verification results](docs/verification.md), and the detailed source-review notes in `docs/`.

Vendor binaries/decompiled code, USB captures and development toolchains are local ignored research artifacts under `research/` and `.tools/`; they are not part of the source deliverable. Captures may include rendered screen content. The Go implementation was written from protocol observations and packet descriptions; it does not load the vendor executable or its libraries.
