// Package example is a portable, compiled-in module template. Its values are
// explicitly synthetic demo data, never real hardware or account metrics.
package example

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"io"
	"github.com/kawapiki/Jonsbo-resurrection/pkg/module"
	"math"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type State struct {
	Demo      bool      `json:"demo"`
	Timestamp time.Time `json:"timestamp"`
	Tick      uint64    `json:"tick"`
	Message   string    `json:"message"`
	Progress  float64   `json:"progress"`
}

type Module struct {
	interval       time.Duration
	mu             sync.RWMutex
	latest         State
	message        string
	ready, running bool
}

var _ module.Module = (*Module)(nil)
var _ module.Renderer = (*Module)(nil)
var _ module.EventHandler = (*Module)(nil)

func New(interval time.Duration) (*Module, error) {
	if interval <= 0 {
		return nil, fmt.Errorf("%w: example interval must be positive", module.ErrInvalid)
	}
	return &Module{interval: interval, message: "Synthetic animation demo"}, nil
}
func (*Module) Descriptor() module.Descriptor {
	return module.Descriptor{ID: "example", Name: "Example animation", Version: "1.0.0", Description: "Synthetic demo data: message events, progress animation and a generated sparkline.", Views: []module.View{{ID: "demo", Width: 640, Height: 180}}}
}

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
		return fmt.Errorf("%w: example already running", module.ErrUnavailable)
	}
	m.running = true
	m.ready = false
	m.mu.Unlock()
	defer func() { m.mu.Lock(); m.running = false; m.mu.Unlock() }()
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for tick := uint64(1); ; tick++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		m.mu.Lock()
		s := State{Demo: true, Timestamp: time.Now().UTC(), Tick: tick, Message: m.message, Progress: float64(tick%101) / 100}
		m.latest = s
		m.ready = true
		m.mu.Unlock()
		if err := publish(s); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// HandleEvent queues a message for the next published tick. Incoming payloads
// are data only: no commands, URLs, files or credentials are interpreted.
func (m *Module) HandleEvent(ctx context.Context, event module.Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if event.Type != "message" {
		return fmt.Errorf("%w: example event type", module.ErrUnsupported)
	}
	if err := module.ValidateEvent(event); err != nil {
		return err
	}
	if !utf8.Valid(event.Payload) {
		return fmt.Errorf("%w: message must be UTF-8", module.ErrInvalid)
	}
	var payload struct {
		Text string `json:"text"`
	}
	dec := json.NewDecoder(bytes.NewReader(event.Payload))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&payload); err != nil {
		return fmt.Errorf("%w: expected message text", module.ErrInvalid)
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("%w: trailing JSON", module.ErrInvalid)
	}
	if strings.TrimSpace(payload.Text) == "" || utf8.RuneCountInString(payload.Text) > 200 {
		return fmt.Errorf("%w: message requires 1–200 characters", module.ErrInvalid)
	}
	m.mu.Lock()
	m.message = payload.Text
	m.mu.Unlock()
	return nil
}

// Render uses the last published state, so repeated renders of a tick are stable.
// The message is exposed in JSON; this intentionally font-free frame demonstrates
// image generation without platform libraries or a USB dependency.
func (m *Module) Render(ctx context.Context, view string) (image.Image, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if view != "demo" {
		return nil, module.ErrNotFound
	}
	m.mu.RLock()
	s, ready := m.latest, m.ready
	m.mu.RUnlock()
	if !ready {
		return nil, module.ErrUnavailable
	}
	frame := image.NewRGBA(image.Rect(0, 0, 640, 180))
	fill := func(rect image.Rectangle, c color.RGBA) {
		draw.Draw(frame, rect, &image.Uniform{C: c}, image.Point{}, draw.Src)
	}
	fill(frame.Bounds(), color.RGBA{18, 24, 32, 255})
	fill(image.Rect(24, 130, 616, 154), color.RGBA{40, 50, 62, 255})
	accent := color.RGBA{uint8(50 + 205*s.Progress), uint8(205 - 125*s.Progress), uint8(220 - 150*s.Progress), 255}
	fill(image.Rect(24, 130, 24+int(592*s.Progress), 154), accent)
	// A synthetic moving wave is the chart/animation extension point.
	for x := 24; x < 616; x++ {
		y := 70 + int(30*math.Sin(float64(x-24)/38+float64(s.Tick)/5))
		fill(image.Rect(x, y, x+1, y+3), accent)
	}
	return frame, nil
}
