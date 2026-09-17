# JONSBO TF3-360SC initial investigation

Date: 2026-09-17

## Observed locally (read-only investigation)

- Vendor program: C:\Program Files (x86)\JONSBO\JONSBO\JONSBO\JONSBO.exe
- JONSBO process running, PID 10340 at inspection.
- Libraries include LibUsbDotNet.LibUsbDotNet.dll, HidLibrary.dll, RJCP.SerialPortStream.dll and FFmpeg bindings.
- Windows USB registry has one TURZX1.0 WinUSB device (VID_1CBE, PID_0035, serial 0628c908f4ca0504) and three TURZX-338inch-r WinUSB devices (VID_43A8, PID_0E61, serials bcd32929c0, bcd32ae5c2, bcd336dbce).
- Vendor config filenames match those four serials.
- Vendor debug.log at 19:03 reports GetAllDevice.count = 4, GetVer()=>turzx_0001_0012, and Send WchCmd.StopVideo.
- Theme directories are named 640480 and 180640; these suggest dimensions but do not yet prove physical panel resolution or orientation.
- USBPcap 1.5.4.0 and Wireshark 4.6.2 appear in installed program registry.
- Get-PnpDevice and Win32_Process CIM queries returned access denied in the sandbox. Registry entries can include historical devices; live enumeration still needs verification.
- No USB commands sent, no captures made, no software stopped or drivers changed.

## Next evidence needed

1. Read vendor assembly metadata and recover display discovery / transfer routines.
2. Verify live device interfaces and endpoints.
3. Capture narrowly scoped display traffic to confirm initialization, image encoding, packet framing, acknowledgements and timing.
4. Prove a single static frame from Go, then independent updates on all four panels.
5. Add a local event API and configurable layouts after transport is verified.

## Proposed app direction (not implemented)

Windows-first Go background app using existing WinUSB interfaces, independent display workers with bounded queues, timeouts and reconnect handling, a renderer, and a loopback API for notifications and agent status events. UI and sensor sources can be added around the proven display transport. Performance targets must be measured against USB and device limits.

## Manufacturer sources

- https://www.jonsbo.com/en/products/TF3-360SC.html
- https://www.jonsbo.com/Upfiles/down/TF3-360SC%20Software%20Manual.pdf

Product page describes four IPS screens and LCD USB connectivity.

## Windows-first follow-up

The user selected Windows as the first platform.

Live Get-PnpDevice enumeration outside the sandbox confirmed all four documented display instances are present with Status OK. The vendor executable is a managed .NET assembly (internal assembly name UsbMonitorL), with obfuscated type and method names. Static metadata identifies imports for WinUsb_Initialize, WinUsb_Free, WinUsb_QueryInterfaceSettings, WinUsb_QueryPipe, WinUsb_ReadPipe, WinUsb_WritePipe and WinUsb_SetPipePolicy.

USBPcap extcap enumeration shows four WinUSB devices on USBPcap4, at capture addresses 6, 9, 10 and 11. Windows device Address properties are port identifiers and must not be substituted for USBPcap capture addresses. A six-second targeted capture attempt did not produce an output file; capture operation remains unverified. No display writes or driver changes were performed.

Related protocol reference: https://github.com/RexPhoe/open-turzx describes a different device, 1CBE:0028. Treat it as a research lead only; compatibility with this cooler is not established.

Go and dotnet were not found on the current PATH or in the standard locations checked. A build toolchain will be needed for implementation.
