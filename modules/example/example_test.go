package example

import (
	"context"
	"encoding/json"
	"errors"
	"image"
	"github.com/kawapiki/Jonsbo-resurrection/pkg/module"
	"strings"
	"testing"
	"time"
)

func TestEventLifecycleAndAnimatedFrames(t *testing.T) {
	m, err := New(10 * time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err = m.Render(ctx, "demo"); !errors.Is(err, module.ErrUnavailable) {
		t.Fatal(err)
	}
	var first image.Image
	count := 0
	err = m.Run(ctx, func(v any) error {
		s := v.(State)
		count++
		if !s.Demo || s.Timestamp.IsZero() || s.Tick != uint64(count) {
			t.Errorf("invalid demo state %+v", s)
		}
		frame, e := m.Render(ctx, "demo")
		if e != nil {
			t.Fatal(e)
		}
		if frame.Bounds() != image.Rect(0, 0, 640, 180) {
			t.Fatal("wrong frame")
		}
		if count == 1 {
			first = frame
			if e = m.HandleEvent(ctx, module.Event{Type: "message", Payload: json.RawMessage(`{"text":"Build complete"}`)}); e != nil {
				t.Fatal(e)
			}
		} else {
			if s.Message != "Build complete" {
				t.Error("event not applied on next tick")
			}
			different := false
			for y := 0; y < 180; y++ {
				for x := 0; x < 640; x++ {
					if first.At(x, y) != frame.At(x, y) {
						different = true
					}
				}
			}
			if !different {
				t.Error("animation did not advance")
			}
			cancel()
		}
		return nil
	})
	if !errors.Is(err, context.Canceled) || count != 2 {
		t.Fatalf("%v publications=%d", err, count)
	}
}

func TestRejectInvalidEvents(t *testing.T) {
	m, _ := New(time.Second)
	for _, payload := range []string{`{}`, `null`, `{"text":""}`, `{"text":3}`, `{"text":"x","extra":true}`, `{"text":"x"} {}`, `{"text":"` + strings.Repeat("é", 201) + `"}`} {
		if err := m.HandleEvent(context.Background(), module.Event{Type: "message", Payload: json.RawMessage(payload)}); !errors.Is(err, module.ErrInvalid) {
			t.Errorf("accepted %q: %v", payload, err)
		}
	}
	if err := m.HandleEvent(context.Background(), module.Event{Type: "command", Payload: json.RawMessage(`{}`)}); !errors.Is(err, module.ErrUnsupported) {
		t.Fatal(err)
	}
	if err := m.HandleEvent(context.Background(), module.Event{Type: "message", Payload: json.RawMessage(`{"text":"` + strings.Repeat("é", 200) + `"}`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := New(0); !errors.Is(err, module.ErrInvalid) {
		t.Fatal("accepted zero interval")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := m.HandleEvent(ctx, module.Event{Type: "message", Payload: json.RawMessage(`{"text":"x"}`)}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	sentinel := errors.New("stop publish")
	if err := m.Run(context.Background(), func(any) error { return sentinel }); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
}
