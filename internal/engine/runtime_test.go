package engine

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

type fakeModule struct {
	id    string
	run   func(context.Context, module.Publish) error
	event func(context.Context, module.Event) error
}

func (f *fakeModule) Descriptor() module.Descriptor {
	return module.Descriptor{ID: f.id, Name: f.id, Version: "test", Views: []module.View{{ID: "main", Width: 2, Height: 2}}}
}
func (f *fakeModule) Run(c context.Context, p module.Publish) error {
	if f.run != nil {
		return f.run(c, p)
	}
	p(map[string]int{"value": 1})
	<-c.Done()
	return c.Err()
}
func (f *fakeModule) Render(context.Context, string) (image.Image, error) {
	return image.NewRGBA(image.Rect(0, 0, 2, 2)), nil
}
func (f *fakeModule) HandleEvent(c context.Context, e module.Event) error {
	if f.event != nil {
		return f.event(c, e)
	}
	return nil
}
func waitState(t *testing.T, r *Runtime, status string) {
	t.Helper()
	until := time.Now().Add(time.Second)
	for time.Now().Before(until) {
		s := r.Snapshot()
		if len(s) > 0 && s[0].Status == status && (status != "running" || len(s[0].Data) > 0) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("state not %s: %+v", status, r.Snapshot())
}
func TestLifecycleRoutingAndCopies(t *testing.T) {
	r := New()
	events := 0
	f := &fakeModule{id: "hardware", event: func(context.Context, module.Event) error { events++; return nil }}
	if err := r.Register(f); err != nil {
		t.Fatal(err)
	}
	if r.Register(f) == nil {
		t.Fatal("duplicate accepted")
	}
	if err := r.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer r.Close(context.Background())
	waitState(t, r, "running")
	s := r.Snapshot()
	s[0].Data[0] = '!'
	s[0].Module.Views[0].Width = 99
	if !json.Valid(r.Snapshot()[0].Data) || r.Snapshot()[0].Module.Views[0].Width != 2 {
		t.Fatal("snapshot aliases runtime state")
	}
	if _, err := r.Frame(context.Background(), "hardware", "main"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Frame(context.Background(), "hardware", "missing"); !errors.Is(err, module.ErrNotFound) {
		t.Fatal(err)
	}
	if err := r.Event(context.Background(), "hardware", module.Event{Type: "message", Payload: json.RawMessage(`{}`)}); err != nil || events != 1 {
		t.Fatal(err)
	}
	if err := r.Event(context.Background(), "hardware", module.Event{Type: "message", Payload: json.RawMessage(`{`)}); !errors.Is(err, module.ErrInvalid) {
		t.Fatal(err)
	}
	if err := r.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitState(t, r, "stopped")
	if r.Start(context.Background()) == nil {
		t.Fatal("runtime restarted unexpectedly")
	}
}
func TestSlowSubscriberKeepsLatestAndRejectsHugeState(t *testing.T) {
	ready := make(chan module.Publish, 1)
	r := New()
	r.Register(&fakeModule{id: "demo", run: func(ctx context.Context, p module.Publish) error { ready <- p; <-ctx.Done(); return ctx.Err() }})
	updates, unsubscribe := r.Subscribe()
	defer unsubscribe()
	r.Start(context.Background())
	defer r.Close(context.Background())
	p := <-ready
	for i := 0; i < 100; i++ {
		if err := p(i); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case states := <-updates:
		if string(states[0].Data) != "99" {
			t.Fatalf("latest snapshot lost: %s", states[0].Data)
		}
	case <-time.After(time.Second):
		t.Fatal("no update")
	}
	if err := p(strings.Repeat("x", module.MaxStateBytes)); !errors.Is(err, module.ErrInvalid) {
		t.Fatal("oversize accepted", err)
	}
}
func TestFailedModuleDoesNotStopOthers(t *testing.T) {
	r := New()
	r.Register(&fakeModule{id: "broken", run: func(context.Context, module.Publish) error { panic("boom") }})
	r.Register(&fakeModule{id: "working"})
	r.Start(context.Background())
	defer r.Close(context.Background())
	waitState(t, r, "failed")
	until := time.Now().Add(time.Second)
	for time.Now().Before(until) {
		s := r.Snapshot()
		if s[1].Status == "running" && json.Valid(s[1].Data) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("healthy module stalled")
}
func TestCloseBeforeStart(t *testing.T) {
	r := New()
	if err := r.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.Start(context.Background()) == nil {
		t.Fatal("closed runtime started")
	}
}
