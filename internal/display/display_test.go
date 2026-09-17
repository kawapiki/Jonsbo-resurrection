package display

import (
	"context"
	"encoding/hex"
	"errors"
	"image"
	"io"
	"os"
	"testing"
	"time"
)

type fakeUSB struct {
	writes      [][]byte
	replies     [][]byte
	short       bool
	shortAt     int
	failRead    bool
	pending     [][]byte
	readTimeout time.Duration
}

func (f *fakeUSB) SetReadTimeout(d time.Duration) error { f.readTimeout = d; return nil }

func (f *fakeUSB) Write(p []byte) (int, error) {
	f.writes = append(f.writes, append([]byte(nil), p...))
	if f.short || (f.shortAt > 0 && len(f.writes) == f.shortAt) {
		return len(p) - 1, nil
	}
	return len(p), nil
}

func TestFanFullFrameAndStaleReplyDrain(t *testing.T) {
	reset := fixture("cc554b8ea0273d3bc540a02c25713ecb481afd9e7f8c146f63dce739c7d800dd")
	frame := fixture("cc55216c53d74131cd90a02c25713ecb481afd9e7f8c146f63dce739c7d800dd")
	f := &fakeUSB{replies: [][]byte{frame, reset, frame}}
	r, err := Send(context.Background(), f, "fan", image.NewRGBA(image.Rect(0, 0, 180, 640)))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.writes) != 92 || r.BytesWritten != 368704 || len(f.replies) != 0 {
		t.Fatalf("incomplete transaction: %+v writes=%d", r, len(f.writes))
	}
	for i, p := range f.writes[2:] {
		if len(p) != 4096 {
			t.Fatalf("chunk %d length %d", i, len(p))
		}
	}
	f = &fakeUSB{shortAt: 4, replies: [][]byte{reset}}
	if _, err := Send(context.Background(), f, "fan", image.NewRGBA(image.Rect(0, 0, 180, 640))); !errors.Is(err, io.ErrShortWrite) || len(f.writes) != 4 {
		t.Fatalf("continued partial frame: writes=%d err=%v", len(f.writes), err)
	}
}

func TestEmptyReadsAreBounded(t *testing.T) {
	f := &fakeUSB{replies: [][]byte{{}, {}, {}, {}, {}}}
	s := session{ctx: context.Background(), transport: f, result: &Result{}}
	if _, err := s.read(512); !errors.Is(err, io.ErrUnexpectedEOF) || len(f.replies) != 1 {
		t.Fatalf("unbounded empty read: %v", err)
	}
}
func (f *fakeUSB) Read(p []byte) (int, error) {
	if len(f.pending) > 0 {
		n := copy(p, f.pending[0])
		f.pending = f.pending[1:]
		return n, nil
	}
	if f.readTimeout == 100*time.Millisecond {
		return 0, os.ErrDeadlineExceeded
	}
	if f.failRead {
		return 0, io.ErrUnexpectedEOF
	}
	if len(f.replies) == 0 {
		return 0, io.EOF
	}
	n := copy(p, f.replies[0])
	f.replies = f.replies[1:]
	return n, nil
}

func TestOldFanSuccessCannotAcknowledgeNewFrame(t *testing.T) {
	oldReset := fixture("cc554b8ea0273d3bc540a02c25713ecb481afd9e7f8c146f63dce739c7d800dd")
	oldFrame := fixture("cc55216c53d74131cd90a02c25713ecb481afd9e7f8c146f63dce739c7d800dd")
	f := &fakeUSB{pending: [][]byte{oldReset, oldFrame}}
	_, err := Send(context.Background(), f, "fan", image.NewRGBA(image.Rect(0, 0, 180, 640)))
	if err == nil {
		t.Fatal("stale acknowledgements falsely confirmed a new frame")
	}
	if len(f.writes) != 2 {
		t.Fatalf("sent frame without a fresh reset reply: %d writes", len(f.writes))
	}
	if f.readTimeout != 2*time.Second {
		t.Fatal("normal transfer timeout was not restored")
	}
}
func fixture(s string) []byte { b, _ := hex.DecodeString(s); return b }

func TestShortWriteStopsBeforeReadingOrContinuing(t *testing.T) {
	f := &fakeUSB{short: true}
	_, err := Send(context.Background(), f, "fan", image.NewRGBA(image.Rect(0, 0, 180, 640)))
	if !errors.Is(err, io.ErrShortWrite) || len(f.writes) != 1 {
		t.Fatalf("writes=%d err=%v", len(f.writes), err)
	}
}
func TestCancelledContextDoesNotTouchUSB(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f := &fakeUSB{}
	_, err := Send(ctx, f, "pump", image.NewRGBA(image.Rect(0, 0, 640, 480)))
	if !errors.Is(err, context.Canceled) || len(f.writes) != 0 {
		t.Fatalf("writes=%d err=%v", len(f.writes), err)
	}
}
func TestPumpStopsOnMissingReply(t *testing.T) {
	f := &fakeUSB{failRead: true}
	_, err := Send(context.Background(), f, "pump", image.NewRGBA(image.Rect(0, 0, 640, 480)))
	if !errors.Is(err, io.ErrUnexpectedEOF) || len(f.writes) != 1 {
		t.Fatalf("writes=%d err=%v", len(f.writes), err)
	}
}

func TestPumpSkipsZeroLengthUSBTerminator(t *testing.T) {
	ack := make([]byte, 512)
	copy(ack, []byte{102, 200, 0x78, 0x56, 0x34, 0x12})
	f := &fakeUSB{replies: [][]byte{{}, ack}}
	s := session{ctx: context.Background(), transport: f, result: &Result{}}
	err := s.pumpExchange([]byte{1}, 102, 0x12345678)
	if err != nil || len(f.writes) != 1 {
		t.Fatalf("writes=%d err=%v", len(f.writes), err)
	}
}
func TestPumpWaitsForMatchingOpcodeAndTimestamp(t *testing.T) {
	old := make([]byte, 512)
	copy(old, []byte{102, 200, 0x78, 0x56, 0x34, 0x12})
	wrongTime := make([]byte, 512)
	copy(wrongTime, []byte{101, 200, 0x77, 0x56, 0x34, 0x12})
	ack := make([]byte, 512)
	copy(ack, []byte{101, 200, 0x78, 0x56, 0x34, 0x12})
	f := &fakeUSB{replies: [][]byte{old, {}, wrongTime, ack}}
	s := session{ctx: context.Background(), transport: f, result: &Result{}}
	if err := s.pumpExchange([]byte{1}, 101, 0x12345678); err != nil {
		t.Fatal(err)
	}
	if len(f.replies) != 0 {
		t.Fatal("accepted a stale reply")
	}
	ack[1] = 0xff
	f.replies = [][]byte{ack}
	if err := s.pumpExchange([]byte{1}, 101, 0x12345678); err == nil {
		t.Fatal("accepted device error")
	}
}
func TestInvalidGeometryDoesNotTouchUSB(t *testing.T) {
	f := &fakeUSB{}
	if _, err := Send(context.Background(), f, "fan", image.NewRGBA(image.Rect(0, 0, 640, 180))); err == nil || len(f.writes) != 0 {
		t.Fatal("wrong geometry accepted")
	}
}

func TestFanCommandsMatchCapturedVendorPackets(t *testing.T) {
	// A missing reset reply fails after the two captured initialization writes.
	f := &fakeUSB{}
	_, err := Send(context.Background(), f, "fan", image.NewRGBA(image.Rect(0, 0, 180, 640)))
	if err == nil || len(f.writes) != 2 {
		t.Fatalf("writes=%d err=%v", len(f.writes), err)
	}
	want := [][]byte{
		fixture("aa55767623ac62ee3919010000000000000000000000000000000000000000bb"),
		fixture("aa553a817542544ddb3f010000000000000000000000000000000000000000bb"),
	}
	for i := range want {
		if string(f.writes[i]) != string(want[i]) {
			t.Fatalf("command %d: %x", i, f.writes[i])
		}
	}
}
