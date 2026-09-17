// Package engine manages trusted modules and immutable, bounded state snapshots.
package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"github.com/kawapiki/Jonsbo-resurrection/pkg/module"
	"sort"
	"sync"
	"time"
)

type entry struct {
	implementation module.Module
	state          module.State
}
type Runtime struct {
	mu              sync.RWMutex
	entries         map[string]*entry
	subscribers     map[uint64]chan []module.State
	nextSubscriber  uint64
	started, closed bool
	cancel          context.CancelFunc
	done            chan struct{}
}

func New() *Runtime {
	return &Runtime{entries: map[string]*entry{}, subscribers: map[uint64]chan []module.State{}, done: make(chan struct{})}
}
func (r *Runtime) Register(m module.Module) error {
	if m == nil {
		return fmt.Errorf("%w: nil module", module.ErrInvalid)
	}
	d := m.Descriptor()
	if err := module.Validate(d); err != nil {
		return err
	}
	d.Views = append([]module.View(nil), d.Views...)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started || r.closed {
		return fmt.Errorf("registration is closed")
	}
	if _, ok := r.entries[d.ID]; ok {
		return fmt.Errorf("duplicate module %q", d.ID)
	}
	r.entries[d.ID] = &entry{m, module.State{Module: d, Status: "registered", Data: json.RawMessage("null")}}
	r.notifyLocked()
	return nil
}
func (r *Runtime) Start(parent context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started || r.closed {
		return fmt.Errorf("runtime already started or closed")
	}
	if err := parent.Err(); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(parent)
	r.cancel = cancel
	r.started = true
	var wg sync.WaitGroup
	for id, e := range r.entries {
		e.state.Status = "running"
		wg.Add(1)
		go func(id string, e *entry) { defer wg.Done(); r.run(ctx, id, e) }(id, e)
	}
	r.notifyLocked()
	go func() { wg.Wait(); close(r.done) }()
	return nil
}
func (r *Runtime) run(ctx context.Context, id string, e *entry) {
	var err error
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("module panic: %v", recovered)
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		e.state.Status = "stopped"
		if err != nil && !errors.Is(err, context.Canceled) && ctx.Err() == nil {
			e.state.Status = "failed"
			e.state.Error = err.Error()
		}
		r.notifyLocked()
	}()
	err = e.implementation.Run(ctx, func(value any) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("%w: state must be JSON", module.ErrInvalid)
		}
		if len(encoded) > module.MaxStateBytes {
			return fmt.Errorf("%w: module state exceeds %d bytes", module.ErrInvalid, module.MaxStateBytes)
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.closed || e.state.Status != "running" {
			return module.ErrUnavailable
		}
		e.state.Data = encoded
		e.state.UpdatedAt = time.Now().UTC()
		r.notifyLocked()
		return nil
	})
}
func (r *Runtime) Close(ctx context.Context) error {
	r.mu.Lock()
	if !r.closed {
		r.closed = true
		if r.cancel != nil {
			r.cancel()
		} else {
			close(r.done)
		}
	}
	done := r.done
	r.mu.Unlock()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func clone(s module.State) module.State {
	s.Data = append(json.RawMessage(nil), s.Data...)
	s.Module.Views = append([]module.View(nil), s.Module.Views...)
	return s
}
func (r *Runtime) snapshotLocked() []module.State {
	out := make([]module.State, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, clone(e.state))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Module.ID < out[j].Module.ID })
	return out
}
func (r *Runtime) Snapshot() []module.State {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.snapshotLocked()
}

// Subscribe returns full snapshots, including an initial one. Slow consumers lose
// intermediate updates, never the latest state. Call unsubscribe on every exit path.
func (r *Runtime) Subscribe() (<-chan []module.State, func()) {
	r.mu.Lock()
	id := r.nextSubscriber
	r.nextSubscriber++
	ch := make(chan []module.State, 1)
	r.subscribers[id] = ch
	ch <- r.snapshotLocked()
	r.mu.Unlock()
	var once sync.Once
	return ch, func() { once.Do(func() { r.mu.Lock(); delete(r.subscribers, id); close(ch); r.mu.Unlock() }) }
}
func (r *Runtime) notifyLocked() {
	for _, ch := range r.subscribers {
		select {
		case <-ch:
		default:
		}
		ch <- r.snapshotLocked()
	}
}
func (r *Runtime) lookup(id string) (module.Module, module.State, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[id]
	if !ok {
		return nil, module.State{}, module.ErrNotFound
	}
	return e.implementation, clone(e.state), nil
}
func (r *Runtime) ValidateView(id, view string) error {
	m, s, err := r.lookup(id)
	if err != nil {
		return err
	}
	if _, ok := m.(module.Renderer); !ok {
		return module.ErrUnsupported
	}
	for _, v := range s.Module.Views {
		if v.ID == view {
			return nil
		}
	}
	return module.ErrNotFound
}
func (r *Runtime) Frame(ctx context.Context, id, view string) (im image.Image, err error) {
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if err = r.ValidateView(id, view); err != nil {
		return nil, err
	}
	m, s, err := r.lookup(id)
	if err != nil {
		return nil, err
	}
	if s.Status != "running" {
		return nil, module.ErrUnavailable
	}
	defer func() {
		if recover() != nil {
			im = nil
			err = fmt.Errorf("module render failed")
		}
	}()
	im, err = m.(module.Renderer).Render(ctx, view)
	if err != nil {
		return nil, err
	}
	if im == nil {
		return nil, fmt.Errorf("module returned a nil image")
	}
	for _, v := range s.Module.Views {
		if v.ID == view && (im.Bounds().Dx() != v.Width || im.Bounds().Dy() != v.Height) {
			return nil, fmt.Errorf("module returned wrong frame dimensions")
		}
	}
	return im, nil
}
func (r *Runtime) Event(ctx context.Context, id string, event module.Event) (err error) {
	if err = module.ValidateEvent(event); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	m, s, err := r.lookup(id)
	if err != nil {
		return err
	}
	if s.Status != "running" {
		return module.ErrUnavailable
	}
	h, ok := m.(module.EventHandler)
	if !ok {
		return module.ErrUnsupported
	}
	event.Payload = append(json.RawMessage(nil), event.Payload...)
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("module event handler failed")
		}
	}()
	return h.HandleEvent(ctx, event)
}
