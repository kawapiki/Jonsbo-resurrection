# PawnIO CPU sensor dependency

The unmodified signed `internal/telemetry/resources/AMDFamily17.bin` is from PawnIO.Modules **0.2.11** by namazso. Its LGPL-2.1-or-later license is in `COPYING`. The matching complete module source archive, including headers and build scripts, is provided as `PawnIO.Modules-0.2.11-source.zip`.

- Module SHA-256: `dae74615761b78bdf064dfb3e136252ddcc6fc727d88f14738d0e5800d427a91`
- Official release ZIP SHA-256: `43608cb89bc84247fef1368a139013f7d043e17db6d6c8dfc9b46bf0905a81f4`
- Source: https://github.com/namazso/PawnIO.Modules/releases/tag/0.2.11
- Driver: https://pawnio.eu/

The driver is installed separately, not bundled into this executable. The verified official PawnIO2.2.0 installer SHA-256 is `1f519a22e47187f70a1379a48ca604981c4fcf694f4e65b734aaa74a9fba3032`; its Authenticode signature was valid (namazso.eu) when installed with the user's explicit approval on2026-09-17.

The Go application requests only `ioctl_read_smn` for the temperature register0x59800, with the shared PCI mutex held. The official module includes other entry points; the application does not invoke them. The module is embedded at build time; replace the file and rebuild to relink a modified version, subject to the driver's module-signing requirements. No modifications were made to the upstream module.

Temperature register interpretation was cross-checked with LibreHardwareMonitor's Amd17Cpu implementation:
https://github.com/LibreHardwareMonitor/LibreHardwareMonitor/blob/master/LibreHardwareMonitorLib/Hardware/Cpu/Amd17Cpu.cs
