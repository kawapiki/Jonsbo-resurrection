# Windows installer implementation plan

Goal: ship a Windows Setup.exe that installs and launches the app without manual extraction.

Architecture: Inno Setup 6.7.3 packages the existing explicit release payload.
Per-user installation uses a stable LocalAppData/Programs/JonsboResurrection path,
a stable installer AppId, Start menu shortcut and Windows uninstall registration.
The ZIP remains available for portable use. No runtime installer service or updater.

- [ ] Add a pinned, hash-verified Inno compiler bootstrap and installer build script.
- [ ] Reuse the package manifest by extracting the verified portable ZIP as installer input.
- [ ] Build a minimal Ready/Install flow; preserve app-data and existing startup mode.
- [ ] Stop the existing tray gracefully, wait for release of its mutex, and fail before copying if it cannot stop.
- [ ] Relocate existing startup to the stable installed executable using current app commands and same-user UAC when needed.
- [ ] Uninstall only its own startup entry and installed files; retain user settings.
- [ ] Test fresh install, same-path upgrade, uninstall and settings preservation on isolated CI paths; do not touch physical hardware in CI.
- [ ] Publish Setup.exe and checksum alongside the portable ZIP in a new release.

Review focus: failed/cancelled elevation, another user's credentials, an unrelated
startup entry, locked files and partial upgrades, custom paths, repeated installs,
uninstalling after switching startup to a different portable copy.
