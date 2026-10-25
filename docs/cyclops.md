# Raijintek Cyclops circular display

The Cyclops 360 has a 240 × 240 circular LCD. The controller is USB
`1F3A:B128` and appears in Windows as **USB LCD BOX**, using the built-in HID
driver. This implementation does not replace that driver or change firmware.

Connect USB and the cooler's normal power, then restart the tray controller so
it discovers the new screen. In Display Studio select **Cyclops**. Use the same
background upload, widget inspector, saved layouts, rotation and **Save & apply**
workflow as the other displays. Offline design also has a **Cyclops 240 × 240**
format selector.

Ten native circular presets are included: Thermal dial, Graphics dial, Twin
temperatures, Memory orbit, Processor pulse, System orbit, OpenAI orbit, Claude
orbit, AI duet, and System + AI. Rim ring, Compact number and Mini graph widgets
can be combined with the existing Radial gauge and Centered value widgets.
Rim rings leave their centers transparent; mini graphs show two minutes with
gaps where samples are missing. Text stays inside the visible circle, with black
pixels beyond its edge. Missing readings remain `--`.

AI widgets have **Circular activity + quota rings** and **Small provider status
strip** content options. Both use the same provider status, session activity,
quota selection and token counts as the existing rectangular AI widgets. Only
presentation changes: short status labels, weekly and five-hour quota rings,
and compact token counts. Local Claude quota fallback remains labelled local.
Existing provider connections and observers work as before; no additional
hooks, sign-in or setup are required for the Cyclops.

Selecting a redesigned preset creates a new editable draft. Previously saved
layouts keep their original composition; choose a new preset to use the redesign.

The default hardware dashboard shows CPU/GPU temperatures. If a CPU sensor is
unavailable, pick a load or memory preset. Existing sensor driver requirements
still apply; the display driver itself does not require replacing a USB driver.

The cooler tested has no hardware serial. Its `RJK-...` identifier is derived
from the Windows HID instance. Keep it on the same USB port to retain saved
assignments; changing ports can create a new identity.

## Verification and operating behavior

On 2026-10-25 Windows reported HID output reports of 1025 bytes and feature
reports of 9 bytes, including the Windows report-ID byte. Native frame transfers
completed in approximately 38–78 ms. The user confirmed the cyan CPU dial was
visible, then confirmed the violet memory dial stayed steady with one update per
second and a held-open connection. Single-frame tests followed by closing the
connection produced a black screen. Use `serve --all` or continuous `monitor`,
not `image` or `monitor --once`, for normal Cyclops operation.

The output driver therefore retains a dedicated Cyclops connection across
frames, closes it after a transfer error, and rediscovers/reopens it on the
next refresh. Healthy connections bypass device discovery. It releases all retained connections when monitoring stops.
The default one-second refresh is the hardware-tested cadence. USB completion
does not itself acknowledge visible content; no firmware frame ACK is defined.

## Rendering efficiency

Hardware rings, radial gauges and AI quota rings share one rasterizer; the
hardware dashboard and configurator share the circular mask. Ring distance
checks use squared distances, and only ring pixels need an angle calculation.
The rectangular and circular AI cards share provider status and usage selection.

A local Windows amd64 benchmark (Ryzen 5 9600X, median of three runs) measured:

| Preset | Before consolidation | After consolidation |
| --- | ---: | ---: |
| OpenAI orbit | 1.02 ms/frame | 0.59 ms/frame |
| System + AI | 1.58 ms/frame | 1.10 ms/frame |

Allocation counts were unchanged. These measurements cover rendering, not USB
transfer time or total application CPU usage. Reproduce with
`go test ./modules/configurator -run '^$' -bench BenchmarkCircularRender -benchmem -count 3`.

## Protocol evidence

Source: the official [Cyclops product page](https://raijintek.com/cyclops-360/)
and its [RJK CYCLOPS 1.3.6 download](https://raijintek.com/wp-content/uploads/2025/12/RJK-CYCLOPS-1.3.6.zip).
The installer was extracted without installing it. The `LibUsb` and
`Bitmap2Byte` routines were inspected locally. Vendor binaries and decompiled
source remain in ignored research directories and are not distributed.

- Vendor/product: decimal 7994/45352, hexadecimal `1F3A:B128`.
- Class SET_REPORT: request type `0x21`, request `0x09`, value `0x0300`,
  interface zero, eight-byte payload. The sum of its bytes is `0xFF` modulo 256.
- Brightness/orientation: `81 04 00 04 00 00 00 76` uses zero device rotation
  and vendor-default brightness 4/8. Mounting rotation is applied in software.
- Begin frame: `81 01 00 00 00 00 00 7D`.
- Image: top-down, little-endian RGB565, 240 × 240 × 2 = 115200 bytes,
  followed by 512 zero bytes, sent as 113 writes of 1024 bytes.
- No input acknowledgement is read by the vendor application.
- On the native Windows HID path, prefix report ID zero: 9 bytes for feature
  reports and 1025 bytes for pixel reports. Feature transfers use
  `IOCTL_HID_SET_FEATURE`; pixel reports use overlapped `WriteFile`.
  The mapping was checked against the [libusb Windows HID backend](https://github.com/libusb/libusb/blob/v1.0.26/libusb/os/windows_winusb.c).
- Native transfers have a two-second timeout, cancellation/drain on timeout,
  and short-transfer rejection. No partial frame is retried.

The official USB library was also used for a research comparison. It is not
required by the implementation or included in the application build.
