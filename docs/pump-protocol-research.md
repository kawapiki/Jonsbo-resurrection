# Pump static-image sequence: independent source review

2026-09-17. Read-only review of locally decompiled JONSBO source; no hardware writes, vendor execution, or cryptographic script execution. Line numbers refer to `research/vendor/JONSBO.cs` and its line-preserving partially decoded counterpart.

## Important correction

There is no evidence that opcode 100 is a static-image prepare command or that opcode 114 is its commit command. Opcode 100's caller parses six little-endian 32-bit return values (14806–14813). Opcode 114 occurs in a separate async wrapper (14924), not after the static JPEG sender. Do not insert these into the image path based on guessed names.

The pump's `StopVideo`-equivalent override `ꛏ()` is empty (16964–16984). Fan opcode 0x38 must not be reused for the pump. Pump opcode decimal 38 is used by a **CreateFile** path (39877), and has entirely different meaning.

## Source-grounded static theme sequence

Pump wrapper class is `Ꝑ`, starting at 13589. Its `StartTheme`-equivalent override `\ua97f(Monitor)` starts at 17958. At entry (18425 onward) it:

1. Stores the monitor and clears a host-side collection.
2. Calls `깞()` (17937–17956): sends enum `ꜥ`, decimal **41 / hex 29**, with one-byte argument **0**.
3. Calls `꺉()` (17859–17935): creates a new bitmap of the current monitor dimensions, saves it through its PNG encoder, and sends it using image opcode **102 / hex 66**. Default new 32-bit ARGB bitmap pixels are transparent black; this appears intended to clear a composited layer, but that semantic remains an inference.
4. In the no-video branch it calls `ꕷ(false)` (18366 and 18444 onward), which renders/resizes the current frame, JPEG-encodes it, and sends image opcode **101 / hex 65**. `false` selects JPEG; `true` selects PNG. Frame encoding is rejected if larger than **1048576 bytes** (18507). Image sizing uses `width_real` if positive, otherwise configured width, plus configured height (18468).

Each of the three device sends is a request/response exchange. There is no explicit sleep or commit required between these particular operations in the source. The surrounding theme loop sleeps 1000 ms between rendered frames (18367).

This three-exchange sequence is the strongest source-backed initial candidate for replacing existing displayed content: `29(arg=0)` → full-size transparent PNG (`66`) → desired JPEG (`65`). The source does not establish whether the PNG clear is strictly required on an already clean device, or prove that opcode 29 itself stops all forms of video playback. Capture or visible hardware verification is still needed to establish the minimal sequence.

## Header and frame exchange

Transport class `\ua954` starts at 36389. `ꛏ(byte opcode)` (38915) allocates 500 zero plaintext bytes:

- Byte 0: opcode.
- Bytes 2–3: `1A 6D`.
- Bytes 4–7: low four little-endian bytes of the vendor relative timestamp.
- For one-byte setting calls, byte 8 contains the argument (40548–40566).
- For image calls, bytes 8–11 contain **big-endian encoded-image byte length** (38842 onward).

The timestamp is `Convert.ToInt64((DateTime.UtcNow - DateTime.Today.AddDays(-1)).TotalMilliseconds)`, using a baseline stored when the transport is created (36670, 38905). This is not Unix time. It includes the source's UTC/local-midnight mismatch. No independent timestamp initialization command was identified in this static path.

Header builder 38951–38978 encrypts the 500-byte plaintext with DES CBC, key and IV both ASCII `slv3tuzx` (38987–39017), default PKCS7 padding. This produces 504 ciphertext bytes. It copies that to a zero-initialized 512-byte header and writes `A1 1A` to offsets 510–511. Offsets 504–509 remain zero. Encoded image bytes are appended unencrypted after offset 512.

Image routine `ꛏ(byte[],bool)` at 38750 sends **one combined buffer** of 512 header bytes plus the exact image length through `ꜥ(byte[])` (40787). It does not pad the image to a multiple of 512 and does not send separate begin/end packets. Transport does a write with 2000 ms timeout then reads the response.

## Endpoints and response validation

- OUT endpoint 0x01 is opened using the default bulk writer at 38030.
- IN endpoint 0x81 is opened as **interrupt**, read size 512, at 38081.
- The response reader `Ꝑ()` (37366–37399) allocates 512 bytes, reads with 2000 ms timeout, and returns that raw buffer. No decryption is applied.
- The reader ignores the actual read length and error code. Image code only tests whether the returned array is null; there is no documented success status, opcode echo, or checksum validation in this path.
- Some file-operation routines test response byte 8 equal to decimal 200 (e.g. 39830), but this must **not** be generalized to image responses without evidence.

A replacement should check transport errors and read length, preserve/log the raw reply for research, and distinguish 'USB transaction completed' from 'visually confirmed image'. The decompiled implementation alone cannot supply a reliable positive display ACK constant.
