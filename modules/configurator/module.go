package configurator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/kawapiki/Jonsbo-resurrection/pkg/module"
	"image"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type Source interface{ Snapshot() []module.State }
type Module struct {
	dir          string
	source       Source
	mutation     sync.Mutex // Serialize durable edits without holding mu during output callbacks.
	mu           sync.RWMutex
	layouts      map[string]Layout
	assets       map[string]Asset
	bindings     map[string]module.Assignment
	history      []History
	metrics      []Metric
	running      bool
	maxSampleAge time.Duration
}
type diskState struct {
	Version  int                          `json:"version"`
	Layouts  map[string]Layout            `json:"layouts"`
	Assets   map[string]Asset             `json:"assets"`
	Bindings map[string]module.Assignment `json:"bindings"`
}

func New(dir string, source Source) (*Module, error) {
	if source == nil {
		return nil, invalid("nil hardware state source")
	}
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0700); err != nil {
		return nil, err
	}
	m := &Module{dir: dir, source: source, layouts: map[string]Layout{}, assets: map[string]Asset{}, bindings: map[string]module.Assignment{}, history: []History{}, metrics: metricDefinitions(), maxSampleAge: 3 * time.Second}
	for _, sz := range []struct {
		id string
		h  int
	}{{"default-pump", 480}, {"default-fan", 180}} {
		l := Themes(640, sz.h)[0].Layout
		l.ID = sz.id
		l.Name = "Default " + sz.id[8:]
		m.layouts[l.ID] = l
	}
	b, e := readBounded(filepath.Join(dir, "state.json"), maxStateBytes)
	if os.IsNotExist(e) {
		return m, m.cleanOrphanAssets()
	}
	if e != nil {
		return nil, fmt.Errorf("read configurator state: %w", e)
	}
	var s diskState
	if e := decodeStrict(bytes.NewReader(b), &s); e != nil {
		return nil, fmt.Errorf("invalid saved configurator state: %w", e)
	}
	if s.Version != 1 || s.Layouts == nil || s.Assets == nil || s.Bindings == nil || len(s.Layouts) > maxLayouts || len(s.Assets) > maxAssets || len(s.Bindings) > 128 {
		return nil, invalid("saved configurator state limits/version")
	}
	var total int64
	for id, a := range s.Assets {
		if id != a.ID || !idPattern.MatchString(id) || a.Name == "" || len(a.Name) > 128 || a.Width < 1 || a.Width > 640 || a.Height < 1 || a.Height > 480 || a.FrameCount < 1 || a.FrameCount > maxFrames || a.FPS < 1 || a.FPS > 2 || a.Bytes < 1 || a.Bytes > maxAssetBytes || (a.Kind != "image" && a.Kind != "video") {
			return nil, invalid("saved asset metadata")
		}
		var size int64
		for i := 0; i < a.FrameCount; i++ {
			info, e := os.Stat(m.framePath(id, i))
			if e != nil {
				return nil, fmt.Errorf("read saved asset %s: %w", id, e)
			}
			if !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > maxAssetBytes {
				return nil, invalid("saved asset file")
			}
			size += info.Size()
		}
		if size != a.Bytes {
			return nil, invalid("saved asset size mismatch")
		}
		total += size
	}
	if total > maxTotalBytes {
		return nil, invalid("saved media quota")
	}
	for id, l := range s.Layouts {
		if id != l.ID {
			return nil, invalid("saved layout id mismatch")
		}
		if e := ValidateLayout(l); e != nil {
			return nil, e
		}
		if l.Background.AssetID != "" {
			if _, ok := s.Assets[l.Background.AssetID]; !ok {
				return nil, invalid("saved layout asset missing")
			}
		}
	}
	for serial, a := range s.Bindings {
		if serial == "" || len(serial) > 128 || a.Module != "configurator" || !rotation(a.Rotation) {
			return nil, invalid("saved binding")
		}
		if _, ok := s.Layouts[a.View]; !ok {
			return nil, invalid("saved binding layout missing")
		}
	}
	m.layouts = s.Layouts
	m.assets = s.Assets
	m.bindings = s.Bindings
	return m, m.cleanOrphanAssets()
}

// An interrupted upload can leave frames written before the catalog commit.
// Reclaim only directories with this module's exact generated-ID shape, so
// repeated interrupted uploads cannot silently bypass the durable media quota.
func (m *Module) cleanOrphanAssets() error {
	entries, err := os.ReadDir(filepath.Join(m.dir, "assets"))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		id := entry.Name()
		if len(id) != 38 || id[:6] != "asset-" || !idPattern.MatchString(id) {
			continue
		}
		if _, exists := m.assets[id]; exists {
			continue
		}
		if err := os.RemoveAll(filepath.Join(m.dir, "assets", id)); err != nil {
			return fmt.Errorf("clean interrupted asset upload: %w", err)
		}
	}
	return nil
}
func (*Module) Descriptor() module.Descriptor {
	return module.Descriptor{ID: "configurator", Name: "Display configurator", Version: "1.0.0", Description: "Saved local display layouts, media and live telemetry", Views: []module.View{{ID: "default-pump", Width: 640, Height: 480}, {ID: "default-fan", Width: 640, Height: 180}}}
}
func (m *Module) ValidateView(id string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.layouts[id]; !ok {
		return module.ErrNotFound
	}
	return nil
}
func (m *Module) Bindings() map[string]module.Assignment {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := map[string]module.Assignment{}
	for k, v := range m.bindings {
		out[k] = v
	}
	return out
}
func (m *Module) SetMaxSampleAge(age time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.maxSampleAge = max(age, 3*time.Second)
}
func (m *Module) Render(ctx context.Context, id string) (image.Image, error) { return m.Frame(ctx, id) }
func (m *Module) Frame(ctx context.Context, id string) (image.Image, error) {
	m.mu.RLock()
	l, ok := m.layouts[id]
	m.mu.RUnlock()
	if !ok {
		return nil, module.ErrNotFound
	}
	return m.render(ctx, l, time.Now())
}
func (m *Module) Run(ctx context.Context, publish module.Publish) error {
	if publish == nil {
		return invalid("nil publisher")
	}
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return module.ErrUnavailable
	}
	m.running = true
	m.mu.Unlock()
	defer func() { m.mu.Lock(); m.running = false; m.mu.Unlock() }()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if e := ctx.Err(); e != nil {
			return e
		}
		m.sample(time.Now())
		m.mu.RLock()
		metrics := append([]Metric(nil), m.metrics...)
		m.mu.RUnlock()
		if e := publish(struct {
			Metrics []Metric `json:"metrics"`
		}{metrics}); e != nil {
			return e
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// Metrics are decoded from the already-running hardware module, never a second collector.
func (m *Module) sample(now time.Time) {
	metrics := metricDefinitions()
	values := map[string]*float64{}
	for _, v := range metrics {
		values[v.ID] = nil
	}
	type sensor struct {
		Usage *float64 `json:"usage_percent"`
		Temp  *float64 `json:"temperature_c"`
		Total *uint64  `json:"memory_total_bytes"`
		Used  *uint64  `json:"memory_used_bytes"`
	}
	var s struct {
		CPU    sensor `json:"cpu"`
		Memory struct {
			Total uint64   `json:"total_bytes"`
			Used  uint64   `json:"used_bytes"`
			Usage *float64 `json:"usage_percent"`
		} `json:"memory"`
		GPUs  []sensor `json:"gpus"`
		Disks []struct {
			Total uint64   `json:"total_bytes"`
			Free  uint64   `json:"free_bytes"`
			Usage *float64 `json:"usage_percent"`
		} `json:"disks"`
	}
	available := false
	m.mu.RLock()
	maxAge := m.maxSampleAge
	m.mu.RUnlock()
	for _, st := range m.source.Snapshot() {
		if st.Module.ID == "hardware" && st.Status == "running" && st.Error == "" && !st.UpdatedAt.IsZero() && now.Sub(st.UpdatedAt) <= maxAge && now.Sub(st.UpdatedAt) >= -time.Second {
			available = json.Unmarshal(st.Data, &s) == nil
			break
		}
	}
	giB := float64(uint64(1) << 30)
	maxima := map[string]float64{}
	put := func(k string, v *float64) {
		if v != nil && finite(*v) && *v >= 0 {
			copy := *v
			values[k] = &copy
		}
	}
	number := func(v float64) *float64 { return &v }
	if available {
		put("cpu.usage", s.CPU.Usage)
		put("cpu.temp", s.CPU.Temp)
		put("ram.usage", s.Memory.Usage)
		if s.Memory.Total > 0 && s.Memory.Used <= s.Memory.Total {
			put("ram.free", number(float64(s.Memory.Total-s.Memory.Used)/giB))
			maxima["ram.free"] = float64(s.Memory.Total) / giB
		}
		if len(s.GPUs) > 0 {
			primary := 0
			for i := range s.GPUs {
				if s.GPUs[i].Total != nil && (s.GPUs[primary].Total == nil || *s.GPUs[i].Total > *s.GPUs[primary].Total) {
					primary = i
				}
			}
			g := s.GPUs[primary]
			put("gpu.usage", g.Usage)
			put("gpu.temp", g.Temp)
			if g.Total != nil && *g.Total > 0 && g.Used != nil && *g.Used <= *g.Total {
				put("gpu.vram", number(float64(*g.Used)/giB))
				maxima["gpu.vram"] = float64(*g.Total) / giB
			}
		}
		if len(s.Disks) > 0 {
			d := s.Disks[0]
			if d.Total > 0 && d.Free <= d.Total {
				put("disk.used", d.Usage)
				put("disk.free", number(float64(d.Free)/giB))
				maxima["disk.free"] = float64(d.Total) / giB
			}
		}
	}
	for i := range metrics {
		metrics[i].Value = values[metrics[i].ID]
		if v := maxima[metrics[i].ID]; v > 0 {
			metrics[i].Max = v
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metrics = metrics
	m.history = append(m.history, History{Time: now.UTC(), Values: values})
	start := 0
	for start < len(m.history) && m.history[start].Time.Before(now.Add(-10*time.Minute)) {
		start++
	}
	if len(m.history)-start > 600 {
		start = len(m.history) - 600
	}
	if start > 0 {
		m.history = append([]History(nil), m.history[start:]...)
	}
}
func readBounded(path string, limit int64) ([]byte, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, limit+1))
	if e != nil {
		return nil, e
	}
	if int64(len(b)) > limit {
		return nil, invalid("file exceeds size limit")
	}
	return b, nil
}
func decodeStrict(r io.Reader, dst any) error {
	d := json.NewDecoder(r)
	d.DisallowUnknownFields()
	if e := d.Decode(dst); e != nil {
		return fmt.Errorf("%w: %w", module.ErrInvalid, e)
	}
	if e := d.Decode(new(any)); e != io.EOF {
		return invalid("trailing JSON")
	}
	return nil
}
func (m *Module) stateCopy() diskState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s := diskState{Version: 1, Layouts: map[string]Layout{}, Assets: map[string]Asset{}, Bindings: map[string]module.Assignment{}}
	for k, v := range m.layouts {
		s.Layouts[k] = v
	}
	for k, v := range m.assets {
		s.Assets[k] = v
	}
	for k, v := range m.bindings {
		s.Bindings[k] = v
	}
	return s
}

// Stage in the same directory: a failed replacement never removes the old state.
func (m *Module) stage(s diskState) (string, error) {
	b, e := json.Marshal(s)
	if e != nil {
		return "", e
	}
	if len(b) > maxStateBytes {
		return "", invalid("state quota")
	}
	f, e := os.CreateTemp(m.dir, ".state-*")
	if e != nil {
		return "", e
	}
	path := f.Name()
	success := false
	defer func() {
		f.Close()
		if !success {
			os.Remove(path)
		}
	}()
	if _, e = f.Write(b); e != nil {
		return "", e
	}
	if e = f.Sync(); e != nil {
		return "", e
	}
	if e = f.Close(); e != nil {
		return "", e
	}
	success = true
	return path, nil
}
func (m *Module) commit(path string, s diskState) error {
	if e := os.Rename(path, filepath.Join(m.dir, "state.json")); e != nil {
		return e
	}
	m.mu.Lock()
	m.layouts = s.Layouts
	m.assets = s.Assets
	m.bindings = s.Bindings
	m.mu.Unlock()
	return nil
}
func (m *Module) save(s diskState) error {
	path, e := m.stage(s)
	if e != nil {
		return e
	}
	defer os.Remove(path)
	return m.commit(path, s)
}
func (m *Module) framePath(id string, index int) string {
	return filepath.Join(m.dir, "assets", id, fmt.Sprintf("%03d.frame", index))
}
func sortedLayouts(s diskState) []Layout {
	out := make([]Layout, 0, len(s.Layouts))
	for _, l := range s.Layouts {
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
func sortedAssets(s diskState) []Asset {
	out := make([]Asset, 0, len(s.Assets))
	for _, a := range s.Assets {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
