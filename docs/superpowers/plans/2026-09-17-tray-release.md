# Tray application and versioned releases

User requests publication to kawapiki/Jonsbo-resurrection, downloadable versioned releases, Windows startup with tray icon, and reuse of their wheelcap signature.

Design: retain CLI/server, add native Windows tray app as no-argument entry point. Tray starts/stops server child, shows status, opens logs/release page and toggles current-user startup. Normal startup uses HKCU Run; optional elevated scheduled task supports the approved CPU sensor. Desktop executable has GUI subsystem, embedded signature icon and version metadata; console executable retained for CLI. App settings/logs/token under LocalAppData, independent of working directory. Release workflow tests/builds/packages on version tags and creates GitHub releases with SHA256 checksums. First release v0.3.0 unless remote existing versions conflict.

Tasks:
- [x] Native tray and application lifecycle, meaningful unit tests and live check.
- [x] Reversible current-user startup registration and elevated temperature mode.
- [x] Reuse signature asset, executable resource/version metadata, public module path.
- [x] Release workflow, docs and packaging; run tests/vet/build, review artifacts.
- [x] Preserve remote history, publish source/tag/release, verify download; enable startup and leave tray dashboards running.

Rulings: user explicitly authorizes release publication and Windows startup, so no repeated permission gate. Preserve existing remote content and history, never force push. Separate tasks delegated under subagent-driven-development skill. Native Win32 UI keeps runtime dependency footprint small. MIT retained from existing draft; no license change requested. Downloaded binaries are unsigned unless a signing certificate is available; do not invent signing identity.

Verification: full Windows tests passed uncached, vet/build passed. GUI resources report version 0.3.0 and the signature icon is embedded as resource 1. Live tray child delivered all four hardware views and elevated CPU temperature. Normal startup enable/status/disable passed; elevated startup registration completed successfully for the current user. Tray private stop exited cleanly. Independent review identified packaging-version mismatch risk; fixed and re-reviewed. Public source keeps the remote MIT license and author. Native visual menu confirmation remains requested from the user; no reboot/logoff is forced for testing.

Publication completed: preserved origin/master history and published v0.3.0 at source commit a532aef. GitHub CI passed Windows Go 1.24.x, Windows stable and portable Ubuntu checks; the release workflow succeeded. Downloaded the public ZIP, verified SHA-256 cf9d6b02cb1d00cff08e69bc5079ac95e1247557a5c98a54c437a90c1d68ef84, checked version/revision, and installed the exact release binary. Manual execution of the registered sign-in task launched the tray and its owned server. A fresh API check on the installed release reported all four hardware assignments live and CPU temperature available. No forced reboot/sign-out was performed. A temporary diagnostic task was removed after verification. The interactive Codex package view redirected test app-data files; verification in the scheduled app's Windows context confirmed the installed app's normal data and credentials.
