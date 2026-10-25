# Windows application

Download **JonsboResurrection-<version>-Setup.exe** from [GitHub Releases](https://github.com/kawapiki/Jonsbo-resurrection/releases/latest), run it, and click **Install**. Setup installs for your account under `%LOCALAPPDATA%\Programs\JonsboResurrection`, adds a Start menu shortcut and an Installed apps entry, then launches the controller. The portable ZIP remains available for manual extraction. No Go installation is required. The app starts hardware monitoring and the local API, then stays in the notification area with the maintainer's neon signature icon. Windows may initially place it in the hidden-icons overflow.

Close the original JONSBO application before driving the screens. Right-click the signature icon for monitoring, log folder, release page, startup, and exit controls. Exiting stops the server owned by this tray process. It does not stop an unrelated standalone server.

## Start with Windows

Use the tray's startup controls to enable or disable launch at sign-in. Ordinary startup is per-user and does not need administrator rights. The installer keeps a stable executable path and preserves an existing startup mode during updates. Portable ZIP users should keep their extracted folder in place and enable startup again after moving it.

CPU temperature needs the separately installed signed PawnIO driver and an elevated process. The elevated startup option registers a task for **your interactive sign-in session**, with highest available privileges; Windows asks for administrator approval during registration. It does not run as SYSTEM, store a password, or install the driver. You can disable it from the tray. Standard users without an administrative token must use normal startup; do not enter another person's credentials to register their task.

The console executable supports the same controls:

```powershell
.\bin\jonsbo.exe startup status
.\bin\jonsbo.exe startup enable
# Run from an administrator terminal, with the desktop EXE preferred for registration:
.\JonsboResurrection.exe startup enable --elevated
.\bin\jonsbo.exe startup disable
.\bin\jonsbo.exe stop --tray
```

Startup registration does not copy or install the app. Use the desktop EXE/tray to register the GUI binary. Registering the console executable runs its tray mode with a console subsystem; use the desktop binary for a windowless start.

## Updating an installed copy

Run the new Setup.exe. Setup closes the existing tray, replaces the installed files, and moves any existing startup registration to the installed executable while retaining its normal/elevated mode. Windows may request same-account UAC approval for elevated startup. Cancelling that approval keeps the previous startup settings; Setup reports the incomplete migration instead of claiming success.

For portable ZIP updates, exit the tray, extract the complete release, run the new copy, and re-enable the same startup mode from its tray.

Keep `%LOCALAPPDATA%\JonsboResurrection` when updating; it contains your saved layouts and display assignments. An older executable can reject layouts containing newer widgets, with an error such as `json: unknown field "provider"`. In that case, update the executable registered for startup rather than deleting the saved layouts. For elevated startup, the registered executable is visible in the Jonsbo task's Actions tab in Task Scheduler.

## Uninstall

Use Windows **Settings → Apps → Installed apps → Jonsbo Resurrection → Uninstall**. Uninstall removes the installed files, shortcut and this copy's startup registration. Saved layouts, assignments and credentials remain in `%LOCALAPPDATA%\JonsboResurrection`; reinstalling reuses them. It does not remove a startup registration subsequently assigned to another portable copy.

## Data and troubleshooting

Tray mode keeps data under `%LOCALAPPDATA%\JonsboResurrection`, regardless of the launch directory. The API token is private local data and is never included in a release. CLI `serve` retains its explicit/working-directory paths. The API remains authenticated and restricted to loopback.

If no screens are attached at launch, the tray server can run with API and sensors only. Connect the cooler, then stop and start monitoring from the tray to discover it. A disconnected selected display is retried by its existing worker. Errors appear in the tray status and log folder. CPU temperature unavailable without elevation is expected; other supported metrics still work.

The application and Setup.exe are not Authenticode signed. Windows may show an unknown-publisher/SmartScreen prompt. Download only from the repository's Releases page and compare the published SHA-256 checksum. The ZIP includes dependency notices and matching signed-module source; keep these alongside the executables when redistributing.
`config.json` in the tray app-data folder is optional. Copy and customize the example config there; when present it controls enabled modules. The tray still fixes token/log paths inside app data. Restart monitoring after editing it.

Builds with the configurator include **Open configurator** in the tray menu. See [the editor guide](configurator.md) for saved overlays, themes and background media.
