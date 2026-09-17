# Fan display protocol research

Static analysis only, 2026-09-17. No vendor methods executed and no USB commands sent during this investigation. Source references below are one-based lines in the locally decompiled `research/vendor/JONSBO.cs`; the partially string-decoded `JONSBO.decoded.cs` preserves those line numbers. These ignored files contain vendor implementation and should not be distributed with the replacement app.

## Device and transport

- `Dev_338Wch` is 180 x 640, `IsWch=true`, `IsPotrit=true` (176983–176991). The supplied hardware inventory identifies these as VID 43A8/PID 0E61, TURZX-338inch-r.
- `Monitor` chooses global class `Ꝼ` for IsWch displays (131217–131220). That implementation owns transport class `\ua929` (26569).
- Transport opens OUT endpoint 0x02 with the default bulk writer and IN endpoint 0x82 as interrupt, read size 32 (35740–35741).
- Writes use 2000 ms timeout (36219 and 36291). Reads request 32 bytes with 100 ms timeout (36006–36009); vendor code does not reliably validate actual read length or errors. Replacement should.
- Serial matching compares the third `#`-separated device-path component case-insensitively (34109).

## Command packets

Packet builder at 35391–35416 creates 32 initially zero bytes:

| Offset | Value |
| --- | --- |
| 0–1 | AA 55 |
| 2–9 | DES encrypted command block |
| 10 | 01 |
| 11–30 | zero |
| 31 | BB |

Plain command block is eight bytes `[opcode, argument, 0, 0, 0, 0, 0, 0]`. Encrypt with single DES ECB key `41 5F D9 FA 13 42 58 B7`. The vendor encryption helper uses PKCS7 padding, but the packet builder copies only the first eight ciphertext bytes. Thus directly encrypting the single eight-byte block without padding gives identical command bytes. Encryption helper: 34814–34858; decryption helper: 34860 onward. No checksum is added.

After a successful command write, vendor sleeps 1 ms then reads 32 bytes (34977 onward). Response bytes 2–9 are DES ECB decrypted with the same key, no padding, leaving other response bytes intact (36017–36021).

| Opcode | Meaning / evidence |
| --- | --- |
| 38 | StopVideo; argument zero; explicit decoded log at 26913–26915 |
| 40 | Firmware/model query, argument zero (27767). Expected decrypted response byte 2 is 41; version is decimal bytes 3 and 4; model byte 5 equal to 4 means `338_Rect` (27590–27755). |
| 58 | ResetMem, argument zero; explicit log at 35088–35090; callable helper at 27548–27567 |
| 54 | One-byte setting, meaning not established (27343–27361) |
| 56 | One-byte setting, meaning not established (27364–27386) |
| 70 | Test-mode command, NOT ordinary initialization: 27544; caller guarded by `Util.testmode` at 24764–24788 |

## Static frame bytes

The static-image override at 29815 onward calls `Util.ꗸ(Bitmap)` and transport `ꜥ(byte[])`. The converter (174034–174054) locks the bitmap as Windows GDI `Format24bppRgb` and copies raw memory. This is **BGR24**, not an encoded BMP/JPEG/PNG file. Row stride is `(width * 3 + 3) & ~3`. For width 180 stride is 540, so 180 x 640 requires exactly 345600 bytes without row padding.

The converter copies rows in increasing memory order from Scan0. A conventional freshly constructed bitmap has top-down logical rows here; physical orientation and color order still warrant a visible calibration pattern. The vendor rendering path supports software rotations (29029, 29087, 29093), but the simple static-image override performs no implicit rotation.

## Frame packetization

`\ua929.ꜥ(byte[])` starts at 35037. It allocates `ceil(payload_length / 480) * 512` initially zero bytes. Each 512-byte record contains:

| Offset | Value |
| --- | --- |
| 0–1 | AA 55 |
| 2–9 | zero (no encryption on frame data) |
| 10 | 02 |
| 11–12 | one-based record index, big endian |
| 13–30 | zero |
| 31 | BB |
| 32–511 | next 480 bytes of BGR image data |

Header assignments are at 35137–35143, data copies at 35083 / 35102. Final partial record would be zero padded. There is no checksum, overall payload length field, frame dimension field, or final-record marker in this builder. The device knows its dimensions and packet count.

For this display: 345600 image bytes = 720 records = 368640 transfer bytes. The downstream writer (35235 onward) writes the combined records in **4096-byte USB transfers** (35378), which produces exactly 90 writes for a full fan frame. The general vendor chunker reuses its final buffer and would leave previous data in its tail for other sizes; that case does not occur for the 180 x 640 frame.

After the entire frame, vendor sleeps 1 ms then reads one 32-byte response and decrypts bytes 2–9. If decrypted byte 2 equals 60, it sends ResetMem and sleeps 300 ms (35110–35129). Meaning of other response codes, including the successful-frame value, is not established. Do not invent a success ACK constant. Vendor itself mostly treats USB write success as success.

## Initialization sequence and limits of evidence

Normal constructor opens transport and queries firmware/model (26664–26689). Initialization state machine queries firmware and may reopen/retry (24694–24709). The normal StartTheme path calls ResetMem, waits 200 ms, then sends the static frame (29803–29806 and 29681–29686). StopVideo is a separate implemented command; its location in the generic UI switching sequence was not fully traced.

A conservative replacement sequence for a single static image is: open this serial's interface, optionally query and validate model, StopVideo, ResetMem, wait 200 ms, write 90 frame transfers, read and decode the response. This combines individually evidenced display-only commands; exact minimum required initialization remains hardware/capture verification work. Avoid opcode 70 and unknown settings.

The recovered packet layout is strongly grounded in source but has not been confirmed by a USB capture or an actual replacement-client display test. No fan-speed or pump-control commands were found or needed in this path.

