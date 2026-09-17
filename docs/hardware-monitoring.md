# Live hardware monitoring (Windows)

The `stats` command emits newline-delimited JSON; `monitor` renders the same snapshots on the USB screens. Neither launches the JONSBO application. CPU temperature uses the separately installed signed PawnIO driver and requires administrator access. The collector uses native Windows APIs and the installed AMD graphics driver, with no PowerShell/WMI subprocess per sample and no Go dependencies or CGO.

## Run

Close the original JONSBO application first, then:

```powershell
.\scripts\start-monitor.ps1 -Elevated
# Stop gracefully (may request elevation):
.\scripts\stop-monitor.ps1
```

`Ctrl+C` also stops a foreground monitor. `scripts/start-monitor.ps1 -Elevated` starts it with a hidden window, administrator access for CPU sensors, and logs under `bin/`. Without `-Elevated`, other metrics work but CPU temperature is unavailable. This does not configure Windows startup. Only one monitor can run per Windows session; `stop` lets outstanding USB operations finish or time out before handles close. A screen retains its last image after stopping.

```powershell
.\bin\jonsbo.exe stats --count 5
.\bin\jonsbo.exe stats --count 0 --interval 2s
.\bin\jonsbo.exe monitor --all --duration 30s --preview-dir research/previews/hardware
.\bin\jonsbo.exe monitor --serial FAN_SERIAL_2 --role gpu
.\bin\jonsbo.exe monitor --all --once
```

Default sampling is one second, minimum 500 ms. CPU usage needs two samples; the first published snapshot follows one sampling interval. Each screen has one pending snapshot, replaced with the newest if sending is slower than sampling. Failed USB transactions are abandoned and retried with a fresh connection on the next snapshot. The existing verified complete-frame initialization is retained on every update. This favors recovery over maximum frame rate; the example module now demonstrates dashboard-rate animation; video-rate playback is not implemented.

## Installed display assignments

With `--all`, pump displays get the summary and fans get CPU, GPU, RAM in sorted serial order. This installation maps to:

| Display | Serial | Contents |
| --- | --- | --- |
| Pump / 1 | PUMP_SERIAL | CPU, GPU, RAM summary and clock |
| Fan / 2 | FAN_SERIAL_1 | CPU usage and Tctl/Tdie temperature |
| Fan / 3 | FAN_SERIAL_2 | GPU usage, temperature, power, dedicated VRAM used/total |
| Fan / 4 | FAN_SERIAL_3 | RAM usage and capacity |

Fans retain the confirmed clockwise 90-degree correction for the horizontal installation. The logical layouts are 640x180 for fans and 640x480 for the pump. For another machine, assignments follow its sorted serials. Changes to the connected device set require restarting the monitor; previously selected serials are re-enumerated for reconnects.

## Readings and limitations

- CPU usage: busy time from `GetSystemTimes` deltas, including all 12 logical processors of this Ryzen 5 9600X. This is processor busy time, not the frequency-adjusted processor utility sometimes shown in Task Manager. The current implementation is intended for machines with up to 64 logical processors.
- RAM: physical total/available from `GlobalMemoryStatusEx`, expressed in GiB. Hardware-reserved memory is excluded by Windows.
- AMD GPU: read-only ADL2 PMLog queries from the installed system `atiadlxx.dll`. Physical adapters are deduplicated; the discrete Radeon RX 7900 XTX is selected for the dashboard ahead of the integrated Radeon. JSON includes both adapters.
- Dedicated VRAM: ADL memory-info capacity and DedicatedVRAMUsage usage, converted to bytes; screens show GiB. The RX7900XTX driver reports25753026560 bytes (23.984GiB, rounded to24.0 on screen). This is dedicated memory, not Windows shared system-memory allowance. JSON provides nullable `memory_total_bytes` and `memory_used_bytes` independently.
- Supported GPU readings: usage, edge temperature, hotspot temperature, graphics clock, power, GPU fan RPM. Power prefers total board power, falling back to ASIC power. On an integrated GPU, ASIC power can cover the APU domain and is not necessarily isolated GPU board power. GPU fan RPM is not AIO fan RPM.
- CPU temperature: Ryzen Tctl/Tdie through PawnIO2.2.0 and its signed AMDFamily17 module0.2.11. Currently enabled only for Raphael (family19h/model61h) and Granite Ridge (family1Ah/model44h); live verified on the Ryzen5 9600X. Requires running as administrator. Reads only SMN0x59800 with the shared PCI mutex held, applies range/TJ_SEL adjustment, and rejects invalid values. No ACPI temperature substitution. AIO pump speed, AIO fan speed, CPU power, NVIDIA and Intel GPU collectors are not implemented.
- Missing/invalid readings become JSON `null` and screen `--`. A measured zero remains zero. Sensor failures are sampled afresh instead of retaining an old value as current.
- Native AMD driver calls have no cancellation API. A hung graphics driver could stall collection; process isolation would be needed for a hard timeout. Normal USB transfers have timeouts, and each screen runs independently.

## Verification

Automated checks cover CPU idle/busy deltas, reset/invalid counters, RAM capacity arithmetic, missing values, AMD sensor mapping and sanity bounds, environment-independent system DLL paths, dashboard dimensions/opacity, option validation, and exclusive/graceful monitor control. The full previous USB protocol suite still passes.

Live checks on 2026-09-17: CPU and RAM changed across snapshots; RX 7900 XTX returned plausible changing utilization, 56 C edge / 62 C hotspot, clocks, approximately 52-53 W board power, and 0 RPM while its fan was stopped. Three fan and pump USB sends succeeded during a 15-second run, followed by an extended monitoring run. These observations are examples, not performance or accuracy guarantees. Physical dashboard confirmation is requested separately from USB acknowledgement.

API references: [GetSystemTimes](https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-getsystemtimes), [GlobalMemoryStatusEx](https://learn.microsoft.com/en-us/windows/win32/api/sysinfoapi/nf-sysinfoapi-globalmemorystatusex), [AMD ADL headers](https://github.com/GPUOpen-LibrariesAndSDKs/display-library/tree/master/include).

Final live control check: a second monitor was rejected, the stop command shut down the active process, and no USB errors were logged. Native pointer lifetimes and GDI/control-handle cleanup were also reviewed.

## CPU temperature verification and startup

After explicit user approval, the official signed PawnIO2.2.0 driver was installed successfully. It persists in Windows and can be removed through Installed Apps; the Go monitor does not install or alter driver access permissions itself. Normal users receive Access denied, leaving temperature null. Elevated read-only samples measured47.625,47.0,46.375 C on the Ryzen5 9600X. An elevated15-second monitor test exited successfully with all four displays acknowledging updates and no sensor warnings. The pump and CPU layouts show temperature; pump footer and GPU layout show VRAM used/total.

Use `scripts/start-monitor.ps1 -Elevated` for all readings, and `scripts/stop-monitor.ps1` to stop it. These may show Windows UAC prompts. From an administrator terminal, the ordinary `monitor`, `stats`, and `stop` commands work directly. `stats --output FILE` writes JSON lines to a file, and `monitor --log-file FILE` writes status/warnings for a hidden process. GPU clocks and hotspot/RPM remain in JSON even though the compact GPU screen now dedicates its second detail row to VRAM.

Dependency source, license, hashes and provenance are in `third_party/pawnio/README.md`. Unit tests cover temperature register decoding and missing VRAM formatting in addition to the original suite.


The serial labels in this public guide are placeholders. Run `jonsbo list` to obtain your own unit's serials. The module runtime and API now share this collector; see architecture.md.
