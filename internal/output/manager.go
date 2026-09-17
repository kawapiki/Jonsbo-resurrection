// Package output routes logical module views to physical displays.
package output

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"github.com/kawapiki/Jonsbo-resurrection/internal/device"
	"github.com/kawapiki/Jonsbo-resurrection/internal/display"
	"github.com/kawapiki/Jonsbo-resurrection/internal/imageutil"
	"github.com/kawapiki/Jonsbo-resurrection/pkg/module"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type Source interface {
	ValidateView(string, string) error
	Frame(context.Context, string, string) (image.Image, error)
}
type Driver interface {
	Send(context.Context, device.Device, image.Image) error
}
type Options struct {
	Assignments map[string]module.Assignment
	PreviewDir  string
	OnChange    func(module.Display)
}
type screen struct {
	device device.Device
	state  module.Display
	saved  bool
}
type Manager struct {
	mu      sync.RWMutex
	source  Source
	driver  Driver
	screens map[string]*screen
	options Options
	running bool
}

var serialPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func New(source Source, driver Driver, devices []device.Device, options Options) (*Manager, error) {
	m := &Manager{source: source, driver: driver, screens: map[string]*screen{}, options: options}
	ds := append([]device.Device(nil), devices...)
	sort.Slice(ds, func(i, j int) bool { return ds[i].Serial < ds[j].Serial })
	fan := 0
	for _, d := range ds {
		key := strings.ToUpper(d.Serial)
		if !serialPattern.MatchString(d.Serial) || m.screens[key] != nil {
			return nil, fmt.Errorf("%w: duplicate or invalid serial", module.ErrInvalid)
		}
		a := module.Assignment{Module: "hardware", View: "summary"}
		switch d.Kind {
		case "fan":
			a.View = []string{"cpu", "gpu", "memory"}[fan%3]
			a.Rotation = 90
			fan++
		case "pump":
		default:
			return nil, fmt.Errorf("%w: display kind", module.ErrInvalid)
		}
		if configured, ok := options.Assignments[key]; ok {
			a = configured
		}
		if err := m.validate(a); err != nil {
			return nil, err
		}
		m.screens[key] = &screen{device: d, state: module.Display{Serial: d.Serial, Kind: d.Kind, Assignment: a, Status: "waiting"}}
	}
	for serial := range options.Assignments {
		if m.screens[serial] == nil {
			return nil, fmt.Errorf("%w: configured display %s is not selected", module.ErrNotFound, serial)
		}
	}
	if options.PreviewDir != "" {
		if err := os.MkdirAll(options.PreviewDir, 0755); err != nil {
			return nil, err
		}
	}
	return m, nil
}
func (m *Manager) validate(a module.Assignment) error {
	if a.Rotation != 0 && a.Rotation != 90 && a.Rotation != 180 && a.Rotation != 270 {
		return fmt.Errorf("%w: rotation", module.ErrInvalid)
	}
	return m.source.ValidateView(a.Module, a.View)
}
func (m *Manager) Assign(serial string, a module.Assignment) error {
	if err := m.validate(a); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.screens[strings.ToUpper(serial)]
	if s == nil {
		return module.ErrNotFound
	}
	s.state.Assignment = a
	s.state.Status = "waiting"
	s.state.Error = ""
	s.saved = false
	return nil
}
func (m *Manager) Displays() []module.Display {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]module.Display, 0, len(m.screens))
	for _, s := range m.screens {
		out = append(out, s.state)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Serial < out[j].Serial })
	return out
}
func (m *Manager) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		return fmt.Errorf("invalid refresh interval")
	}
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return fmt.Errorf("display manager already running")
	}
	m.running = true
	m.mu.Unlock()
	var wg sync.WaitGroup
	for _, s := range m.screens {
		wg.Add(1)
		go func(s *screen) {
			defer wg.Done()
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				if ctx.Err() != nil {
					return
				}
				m.send(ctx, s)
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}(s)
	}
	wg.Wait()
	return nil
}
func (m *Manager) Once(ctx context.Context) error {
	var wg sync.WaitGroup
	errs := make(chan error, len(m.screens))
	for _, s := range m.screens {
		wg.Add(1)
		go func(s *screen) { defer wg.Done(); errs <- m.send(ctx, s) }(s)
	}
	wg.Wait()
	close(errs)
	var collected []error
	for err := range errs {
		if err != nil {
			collected = append(collected, err)
		}
	}
	return errors.Join(collected...)
}
func (m *Manager) send(ctx context.Context, s *screen) (err error) {
	m.mu.RLock()
	a := s.state.Assignment
	saved := s.saved
	m.mu.RUnlock()
	defer func() {
		m.mu.Lock()
		previous := s.state.Status
		previousError := s.state.Error
		// A frame already in flight may finish after reassignment. Do not report it
		// as confirmation of the new assignment.
		if s.state.Assignment != a {
			m.mu.Unlock()
			return
		}
		if err == nil {
			s.state.Status = "live"
			s.state.LastSuccess = time.Now().UTC()
			s.state.Error = ""
			s.saved = true
		} else if !errors.Is(err, context.Canceled) {
			s.state.Status = "error"
			s.state.Error = err.Error()
		}
		state := s.state
		changed := previous != state.Status || previousError != state.Error
		m.mu.Unlock()
		if changed && m.options.OnChange != nil {
			m.options.OnChange(state)
		}
	}()
	im, err := m.source.Frame(ctx, a.Module, a.View)
	if err != nil {
		return err
	}
	if !saved && m.options.PreviewDir != "" {
		f, e := os.Create(filepath.Join(m.options.PreviewDir, s.device.Serial+"-"+a.Module+"-"+a.View+".png"))
		if e != nil {
			return e
		}
		e = png.Encode(f, im)
		ce := f.Close()
		if e != nil {
			return e
		}
		if ce != nil {
			return ce
		}
	}
	w, h, err := display.Dimensions(s.device.Kind)
	if err != nil {
		return err
	}
	if a.Rotation == 90 || a.Rotation == 270 {
		w, h = h, w
	}
	fitted, err := imageutil.Fit(im, w, h)
	if err != nil {
		return err
	}
	native, err := imageutil.Rotate(fitted, a.Rotation)
	if err != nil {
		return err
	}
	return m.driver.Send(ctx, s.device, native)
}
