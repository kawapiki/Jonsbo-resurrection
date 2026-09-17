//go:build windows

// Package hardware adapts the existing Windows telemetry and dashboard renderer.
package hardware

import (
	"context"
	"fmt"
	"image"
	"github.com/kawapiki/Jonsbo-resurrection/internal/dashboard"
	"github.com/kawapiki/Jonsbo-resurrection/internal/telemetry"
	"github.com/kawapiki/Jonsbo-resurrection/pkg/module"
	"sync"
	"time"
)

type collector interface {
	Sample() telemetry.Snapshot
	Close()
}

type Module struct {
	interval   time.Duration
	open       func() collector
	mu         sync.RWMutex
	latest     telemetry.Snapshot
	lastSample time.Time
	ready      bool
	running    bool
}

var _ module.Module = (*Module)(nil)
var _ module.Renderer = (*Module)(nil)

func New(interval time.Duration) (*Module, error) {
	if interval < 500*time.Millisecond {
		return nil, fmt.Errorf("%w: hardware interval must be at least 500ms", module.ErrInvalid)
	}
	return &Module{interval: interval, open: func() collector { return telemetry.New() }}, nil
}

func (*Module) Descriptor() module.Descriptor {
	return module.Descriptor{ID: "hardware", Name: "System hardware", Version: "1.0.0", Description: "Live Windows CPU, memory and GPU telemetry; unavailable metrics remain null.", Views: []module.View{
		{ID: "summary", Width: 640, Height: 480}, {ID: "cpu", Width: 640, Height: 180}, {ID: "gpu", Width: 640, Height: 180}, {ID: "memory", Width: 640, Height: 180},
	}}
}

// Run opens native resources only while running. The discarded first sample
// establishes counter baselines; the first published sample spans a full interval.
func (m *Module) Run(ctx context.Context, publish module.Publish) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if publish == nil {
		return fmt.Errorf("%w: nil publisher", module.ErrInvalid)
	}
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return fmt.Errorf("%w: hardware already running", module.ErrUnavailable)
	}
	m.running = true
	m.ready = false
	m.mu.Unlock()
	defer func() { m.mu.Lock(); m.running = false; m.ready = false; m.mu.Unlock() }()
	c := m.open()
	defer c.Close()
	c.Sample()
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		s := c.Sample()
		if err := ctx.Err(); err != nil {
			return err
		}
		m.mu.Lock()
		m.latest = s
		m.lastSample = time.Now()
		m.ready = true
		m.mu.Unlock()
		if err := publish(s); err != nil {
			return err
		}
	}
}

func (m *Module) Render(ctx context.Context, view string) (image.Image, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch view {
	case "summary", "cpu", "gpu", "memory":
	default:
		return nil, fmt.Errorf("%w: hardware view", module.ErrNotFound)
	}
	m.mu.RLock()
	s, ready, completed := m.latest, m.ready, m.lastSample
	m.mu.RUnlock()
	// Keep the last snapshot available to state clients, but never render it as
	// current after collection stalls. The clock measures completion, not a
	// potentially old sensor-provided timestamp.
	freshness := max(3*m.interval, 3*time.Second)
	if !ready || time.Since(completed) > freshness {
		return nil, module.ErrUnavailable
	}
	return dashboard.Render(s, view)
}
