# Verified protocol notes

Tested on the attached JONSBO TF3-360SC, 2026-09-17. Distinguish observations below from historical source-analysis hypotheses in the other research notes.

## Windows interfaces

| Device | USB ID | Interface GUID | OUT | IN |
| --- | --- | --- | --- | --- |
| Pump | 1CBE:0035 | 88BAE032-5A81-49F0-BC3D-A4FF138216D6 | 01 | 81 |
| Fans | 43A8:0E61 | 12345678-1234-1344-1234-123456789ABC | 02 | 82 |

Live WinUSB descriptors reported **bulk for all four endpoints**, with 512-byte maximum packets. The vendor's LibUsbDotNet constructors name the input readers `Interrupt`; that source-level parameter does not override the actual descriptors.

The vendor app owns the device handles exclusively. Closing it resolved `Access is denied`; normal Go device access then worked without elevation. USBPcap capture separately required administrator privileges and a fresh output file. Old investigation helpers were stopped. Captures were restricted to verified display addresses 6, 9, 10 and 11 on USBPcap4. These addresses are session-specific, not permanent IDs.

## Fan requests and responses

Source layout is documented in [fan-protocol-research.md](fan-protocol-research.md). Live vendor captures confirmed 4096-byte transfers containing eight 512-byte records. Each record holds 480 BGR24 bytes with a one-based big-endian record number. A complete 180 × 640 frame is 345600 pixel bytes, 720 records, 368640 transfer bytes.

Commands are 32 bytes: `AA 55`, one DES-ECB encrypted eight-byte command block, type 01 at byte 10, trailing `BB` at byte 31. Key: `41 5F D9 FA 13 42 58 B7`.

Captured stop-video command (hex 38):

```text
aa55767623ac62ee3919010000000000000000000000000000000000000000bb
```

Captured reset-memory command (hex 58):

```text
aa553a817542544ddb3f010000000000000000000000000000000000000000bb
```

Replies use **CC 55 ... DD**, not the request markers. Decrypt bytes 2..9 with the same DES key. Observed decoded first bytes: firmware response 41, reset response 59, successful frame response 62. Source marks 60 as a frame error. Stop-video may leave no distinct immediate acknowledgement; do not assume every read corresponds to the latest command. Vendor startup itself consumed queued replies out of order.

The working Go sequence: drain the input queue until a 100 ms idle timeout (at most 16 reads) → restore the 2 s transfer timeout → StopVideo38 → 1 ms → ResetMem58 → wait for response59 (bounded stale-reply drain) → 200 ms → 90 writes of 4096 bytes → 1 ms → response62. All three displays were visually confirmed; the final added idle-boundary check was then hardware-tested on fan 2. The initial drain matters because fan replies have no transaction ID: an old59/62 pair must not falsely acknowledge a new frame.

Physical installation is landscape. Render 640 × 180, rotate clockwise 90° to native 180 × 640, then serialize BGR rows. The owner confirmed upright numbers and the top calibration stripe after this correction.

## Pump requests and responses

Full source derivation is in [pump-protocol-research.md](pump-protocol-research.md). Pump commands have 500 plaintext bytes, DES-CBC encrypted with key=IV ASCII `slv3tuzx`, PKCS7-padded to 504 bytes, six zero bytes, then `A1 1A`. Plaintext byte 0 is opcode, 2..3 are `1A 6D`, 4..7 are timestamp little-endian; image length is big-endian at 8..11. Append the image bytes after the 512-byte encrypted header without padding the image.

The working sequence is mode command **29 hex** with argument 0, transparent full-size PNG **66 hex**, then desired JPEG **65 hex**. Each exchange is acknowledged separately. Native image size is 640 × 480; JPEG quality 95 was used. No prepare100/commit114 commands are used.

Live replies are 512 bytes:

- Byte 0 echoes command opcode.
- Byte 1 is `C8` on successful operations observed here.
- Bytes 2..5 echo the request timestamp little-endian.
- Successful reply examples start `29c8...`, `66c8...`, `65c8...`.

Zero-length reads occur between full-size replies, consistent with USB bulk transfer termination. They must be skipped with a bound. A failed earlier exchange also left a stale PNG/JPEG reply queued. The Go implementation waits for the matching opcode **and timestamp** and checks status C8, preventing a queued success from acknowledging the wrong command.

Timestamp follows the source's milliseconds from previous local calendar day's midnight interpreted as UTC. It is a request identifier, not Unix time. The Go client calculates it for each command, avoiding the vendor's long-lived baseline field.

## Evidence limits

Vendor baseline/startup captures contained active image traffic from two fan addresses; pump and third fan appeared in descriptors but had no image traffic in those short windows. Pump encoding and third fan transfer were subsequently verified directly by Go transactions and the owner's visual confirmation. Orientation, status values and geometry are verified for this unit; other TURZX devices or firmware versions are not automatically supported.
