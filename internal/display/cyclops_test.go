package display

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"io"
	"testing"
)

type cyclopsUSB struct {
	fakeUSB
	controls     [][]byte
	shortControl bool
}

func (f *cyclopsUSB) Control(p []byte) (int, error) {
	f.controls = append(f.controls, append([]byte(nil), p...))
	if f.shortControl {
		return len(p) - 1, nil
	}
	return len(p), nil
}
func TestCyclopsFrameWireFormat(t *testing.T) {
	f := &cyclopsUSB{}
	im := image.NewRGBA(image.Rect(0, 0, 240, 240))
	im.Set(0, 0, color.RGBA{R: 255, A: 255})
	im.Set(1, 0, color.RGBA{G: 255, A: 255})
	im.Set(2, 0, color.RGBA{B: 255, A: 255})
	result, err := Send(context.Background(), f, "cyclops", im)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.controls) != 2 || !bytes.Equal(f.controls[0], []byte{0x81, 4, 0, 4, 0, 0, 0, 0x76}) || !bytes.Equal(f.controls[1], []byte{0x81, 1, 0, 0, 0, 0, 0, 0x7d}) {
		t.Fatalf("commands %x", f.controls)
	}
	if len(f.writes) != 113 || result.BytesWritten != 115712+16 {
		t.Fatalf("incomplete frame: %+v writes=%d", result, len(f.writes))
	}
	for _, chunk := range f.writes {
		if len(chunk) != 1024 {
			t.Fatal("bad chunk size")
		}
	}
	if !bytes.Equal(f.writes[0][:6], []byte{0, 0xf8, 0xe0, 7, 0x1f, 0}) {
		t.Fatalf("RGB565 %x", f.writes[0][:6])
	}
	if !bytes.Equal(f.writes[112][512:], make([]byte, 512)) {
		t.Fatal("padding")
	}
}
func TestCyclopsStopsOnShortTransferAndCancellation(t *testing.T) {
	im := image.NewRGBA(image.Rect(0, 0, 240, 240))
	f := &cyclopsUSB{fakeUSB: fakeUSB{shortAt: 2}}
	if _, err := Send(context.Background(), f, "cyclops", im); !errors.Is(err, io.ErrShortWrite) || len(f.writes) != 2 {
		t.Fatal(err, len(f.writes))
	}
	f = &cyclopsUSB{shortControl: true}
	if _, err := Send(context.Background(), f, "cyclops", im); !errors.Is(err, io.ErrShortWrite) || len(f.writes) != 0 {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f = &cyclopsUSB{}
	if _, err := Send(ctx, f, "cyclops", im); !errors.Is(err, context.Canceled) || len(f.controls) != 0 {
		t.Fatal(err)
	}
	if _, err := Send(context.Background(), &fakeUSB{}, "cyclops", im); err == nil {
		t.Fatal("missing control transport accepted")
	}
}
