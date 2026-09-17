package protocol

import (
	"bytes"
	"crypto/cipher"
	"crypto/des"
	"encoding/hex"
	"testing"
)

// Independent decoding asserts the vendor's plaintext fields, encryption mode,
// padding and envelope; expectations do not use the packet builder.
func TestPumpImagePacketLayout(t *testing.T) {
	payload := []byte{0xff, 0xd8, 0xff, 0xd9}
	p, err := PumpImage(payload, false, 0x12345678)
	if err != nil {
		t.Fatal(err)
	}
	if len(p) != 516 || !bytes.Equal(p[512:], payload) {
		t.Fatal("image payload changed")
	}
	if !bytes.Equal(p[504:512], []byte{0, 0, 0, 0, 0, 0, 0xa1, 0x1a}) {
		t.Fatalf("envelope %x", p[504:512])
	}
	block, _ := des.NewCipher([]byte("slv3tuzx"))
	plain := make([]byte, 504)
	cipher.NewCBCDecrypter(block, []byte("slv3tuzx")).CryptBlocks(plain, p[:504])
	want := make([]byte, 504)
	copy(want, []byte{101, 0, 26, 109, 0x78, 0x56, 0x34, 0x12, 0, 0, 0, 4})
	copy(want[500:], []byte{4, 4, 4, 4})
	if !bytes.Equal(plain, want) {
		t.Fatalf("plaintext mismatch: %x", plain[:20])
	}
}

func TestPumpRejectsInvalidPayloadSizes(t *testing.T) {
	for _, n := range []int{0, 1048577} {
		if _, err := PumpImage(make([]byte, n), false, 0); err == nil {
			t.Errorf("accepted %d bytes", n)
		}
	}
	if _, err := PumpCommand(41, make([]byte, 493), 0); err == nil {
		t.Fatal("accepted oversized command")
	}
}

func TestFanCommandAndReply(t *testing.T) {
	p := FanCommand(0x38, 0)
	if len(p) != 32 || p[0] != 0xaa || p[1] != 0x55 || p[10] != 1 || p[31] != 0xbb {
		t.Fatalf("header %x", p)
	}
	block, _ := des.NewCipher([]byte{0x41, 0x5f, 0xd9, 0xfa, 0x13, 0x42, 0x58, 0xb7})
	plain := make([]byte, 8)
	block.Decrypt(plain, p[2:10])
	if !bytes.Equal(plain, []byte{0x38, 0, 0, 0, 0, 0, 0, 0}) {
		t.Fatalf("command %x", plain)
	}
	// A literal simulated device response, encrypted independently at its boundary.
	reply := make([]byte, 32)
	reply[0] = 0xcc
	reply[1] = 0x55
	reply[31] = 0xdd
	block.Encrypt(reply[2:10], []byte{0x41, 1, 2, 4, 0, 0, 0, 0})
	d, err := DecodeFanReply(reply)
	if err != nil || d[2] != 0x41 || d[5] != 4 {
		t.Fatalf("reply %x %v", d, err)
	}
	for _, bad := range [][]byte{nil, make([]byte, 31), make([]byte, 32)} {
		if _, err := DecodeFanReply(bad); err == nil {
			t.Fatal("accepted malformed response")
		}
	}
}

func TestFanAcceptsCapturedReply(t *testing.T) {
	// USBPcap4 address 11 IN 0x82, vendor baseline 2026-09-17.
	raw, _ := hex.DecodeString("cc55216c53d74131cd902669941cc3984755ac8302c78ea0e3c629baab8e00dd")
	decoded, err := DecodeFanReply(raw)
	if err != nil {
		t.Fatal(err)
	}
	if decoded[2] != 0x62 {
		t.Fatalf("captured status bytes: %x", decoded[2:10])
	}
}

func TestFanFrameNumberingAndPixelBoundaries(t *testing.T) {
	pixels := make([]byte, 345600)
	pixels[0] = 0x11
	pixels[479] = 0x22
	pixels[480] = 0x33
	pixels[len(pixels)-1] = 0x44
	p, err := FanFrame(pixels)
	if err != nil {
		t.Fatal(err)
	}
	if len(p) != 368640 {
		t.Fatalf("frame length %d", len(p))
	}
	for _, tc := range []struct {
		offset int
		hi, lo byte
	}{{0, 0, 1}, {512, 0, 2}, {719 * 512, 2, 0xd0}} {
		h := p[tc.offset : tc.offset+32]
		want := make([]byte, 32)
		want[0] = 0xaa
		want[1] = 0x55
		want[10] = 2
		want[11] = tc.hi
		want[12] = tc.lo
		want[31] = 0xbb
		if !bytes.Equal(h, want) {
			t.Fatalf("record header %d: %x", tc.offset, h)
		}
	}
	if p[32] != 0x11 || p[511] != 0x22 || p[544] != 0x33 || p[len(p)-1] != 0x44 {
		t.Fatal("pixel boundary corruption")
	}
	if _, err := FanFrame(pixels[:len(pixels)-1]); err == nil {
		t.Fatal("accepted truncated frame")
	}
}
