// Package configurator provides durable, local display compositions and rendering.
package configurator

import (
	"fmt"
	"github.com/kawapiki/Jonsbo-resurrection/pkg/module"
	"math"
	"regexp"
	"strings"
	"time"
)

const (
	maxLayouts    = 32
	maxAssets     = 32
	maxOverlays   = 32
	maxFrames     = 120
	maxAssetBytes = 24 << 20
	maxTotalBytes = 128 << 20
	maxBodyBytes  = 32 << 20
	maxStateBytes = 2 << 20
)

var idPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
var colorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

type Background struct {
	Color   string  `json:"color"`
	AssetID string  `json:"asset_id"`
	Opacity float64 `json:"opacity"`
}
type Overlay struct {
	ID       string  `json:"id"`
	Type     string  `json:"type"`
	Metric   string  `json:"metric"`
	Label    string  `json:"label"`
	X        int     `json:"x"`
	Y        int     `json:"y"`
	W        int     `json:"w"`
	H        int     `json:"h"`
	Color    string  `json:"color"`
	FontSize int     `json:"font_size"`
	Min      float64 `json:"min"`
	Max      float64 `json:"max"`
	Opacity  float64 `json:"opacity"`
}
type Layout struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Width      int        `json:"width"`
	Height     int        `json:"height"`
	Background Background `json:"background"`
	Overlays   []Overlay  `json:"overlays"`
	Rotation   int        `json:"rotation"`
}
type Theme struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Layout      Layout `json:"layout"`
}
type Asset struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	FrameCount int    `json:"frame_count"`
	FPS        int    `json:"fps"`
	Bytes      int64  `json:"bytes"`
}
type Metric struct {
	ID    string   `json:"id"`
	Label string   `json:"label"`
	Unit  string   `json:"unit"`
	Value *float64 `json:"value"`
	Max   float64  `json:"max"`
}
type History struct {
	Time   time.Time           `json:"time"`
	Values map[string]*float64 `json:"values"`
}

func metricDefinitions() []Metric {
	return []Metric{
		{ID: "cpu.usage", Label: "CPU usage", Unit: "%", Max: 100}, {ID: "cpu.temp", Label: "CPU temperature", Unit: "C", Max: 100},
		{ID: "gpu.usage", Label: "GPU usage", Unit: "%", Max: 100}, {ID: "gpu.temp", Label: "GPU temperature", Unit: "C", Max: 100},
		{ID: "gpu.vram", Label: "GPU memory", Unit: "GiB", Max: 1}, {ID: "ram.usage", Label: "RAM usage", Unit: "%", Max: 100},
		{ID: "ram.free", Label: "RAM free", Unit: "GiB", Max: 1}, {ID: "disk.used", Label: "Disk used", Unit: "%", Max: 100},
		{ID: "disk.free", Label: "Disk free", Unit: "GiB", Max: 1},
	}
}
func finite(v float64) bool  { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func opacity(v float64) bool { return finite(v) && v >= 0 && v <= 1 }
func rotation(v int) bool    { return v == 0 || v == 90 || v == 180 || v == 270 }
func invalid(s string) error { return fmt.Errorf("%w: %s", module.ErrInvalid, s) }
func ValidateLayout(l Layout) error {
	if !idPattern.MatchString(l.ID) || strings.TrimSpace(l.Name) == "" || len(l.Name) > 128 {
		return invalid("layout id/name")
	}
	if l.Width != 640 || (l.Height != 480 && l.Height != 180) || !rotation(l.Rotation) {
		return invalid("canvas dimensions/rotation")
	}
	if (l.ID == "default-pump" && l.Height != 480) || (l.ID == "default-fan" && l.Height != 180) {
		return invalid("default view canvas size is fixed")
	}
	if !colorPattern.MatchString(l.Background.Color) || !opacity(l.Background.Opacity) || (l.Background.AssetID != "" && !idPattern.MatchString(l.Background.AssetID)) {
		return invalid("background")
	}
	if len(l.Overlays) > maxOverlays {
		return invalid("at most 32 overlays")
	}
	seen := map[string]bool{}
	metrics := map[string]bool{}
	for _, m := range metricDefinitions() {
		metrics[m.ID] = true
	}
	for _, o := range l.Overlays {
		if !idPattern.MatchString(o.ID) || seen[o.ID] {
			return invalid("overlay id must be unique")
		}
		seen[o.ID] = true
		if o.X < 0 || o.Y < 0 || o.W < 1 || o.H < 1 || o.X > l.Width || o.Y > l.Height || o.W > l.Width-o.X || o.H > l.Height-o.Y {
			return invalid("overlay outside canvas")
		}
		if !colorPattern.MatchString(o.Color) || !opacity(o.Opacity) || o.FontSize < 7 || o.FontSize > 96 || len(o.Label) > 200 {
			return invalid("overlay color, opacity, font size or label")
		}
		switch o.Type {
		case "text":
		case "metric", "line", "pie", "bar":
			if !metrics[o.Metric] {
				return invalid("unknown metric")
			}
		default:
			return invalid("overlay type")
		}
		if !finite(o.Min) || !finite(o.Max) || o.Max <= o.Min || math.Abs(o.Min) > 1e12 || math.Abs(o.Max) > 1e12 {
			return invalid("overlay metric range")
		}
	}
	return nil
}
func Themes(width, height int) []Theme {
	if width != 640 || (height != 480 && height != 180) {
		width, height = 640, 480
	}
	// Coordinates are authored independently for both physical aspect ratios.
	makeOverlay := func(id, typ, metric, label string, x, y, w, h, font int, c string) Overlay {
		return Overlay{ID: id, Type: typ, Metric: metric, Label: label, X: x, Y: y, W: w, H: h, Color: c, FontSize: font, Min: 0, Max: 100, Opacity: 1}
	}
	specs := []struct{ id, name, description, bg, fg, accent string }{
		{"arctic", "Arctic", "Cool typography, CPU history and memory bars.", "#081B29", "#E2F6FF", "#52D8F2"},
		{"ember", "Ember", "Warm circular CPU gauge and amber GPU readout.", "#25110C", "#FFD6AC", "#FF743D"},
		{"paper", "Paper", "Bright editorial statistics with dark teal storage details.", "#F0EDE4", "#243A38", "#147D70"},
		{"orchid", "Orchid", "Violet history panels and oversized memory figures.", "#1B1035", "#EFE4FF", "#BA8CFF"},
		{"terminal", "Terminal", "Green console telemetry with compact status rows.", "#07100C", "#A9FFB5", "#42CF79"},
	}
	out := make([]Theme, 0, 5)
	for i, s := range specs {
		l := Layout{ID: s.id, Name: s.name, Width: width, Height: height, Background: Background{Color: s.bg, Opacity: 1}, Overlays: []Overlay{}}
		add := func(id, typ, metric, label string, x, y, w, h, font int, c string) {
			l.Overlays = append(l.Overlays, makeOverlay(id, typ, metric, label, x, y, w, h, font, c))
		}
		if height == 180 {
			// Fan panels are natively portrait; the logical horizontal canvas
			// needs the same clockwise rotation as the hardware dashboard.
			l.Rotation = 90
			switch i {
			case 0:
				add("title", "text", "", "ARCTIC / SYSTEM", 18, 10, 604, 24, 14, s.fg)
				add("cpu", "metric", "cpu.usage", "CPU", 18, 43, 190, 62, 28, s.accent)
				add("history", "line", "cpu.usage", "CPU HISTORY", 228, 40, 394, 92, 14, s.accent)
				add("ram", "bar", "ram.usage", "MEMORY", 18, 137, 604, 32, 14, s.fg)
			case 1:
				add("cpu", "pie", "cpu.usage", "CPU LOAD", 18, 14, 150, 152, 14, s.accent)
				add("gpu", "metric", "gpu.temp", "GPU TEMP", 200, 24, 420, 64, 28, s.fg)
				add("ram", "bar", "ram.usage", "MEMORY", 200, 110, 420, 50, 14, s.accent)
			case 2:
				add("title", "text", "", "SYSTEM / DAILY REPORT", 22, 12, 596, 22, 14, s.fg)
				add("cpu", "metric", "cpu.usage", "PROCESSOR", 22, 49, 180, 72, 28, s.fg)
				add("ram", "metric", "ram.free", "RAM FREE", 232, 49, 180, 72, 21, s.accent)
				add("disk", "metric", "disk.free", "DISK FREE", 436, 49, 184, 72, 21, s.fg)
				add("storage", "bar", "disk.used", "DISK", 22, 138, 596, 30, 14, s.accent)
			case 3:
				add("ram", "metric", "ram.usage", "MEMORY", 22, 22, 215, 103, 35, s.fg)
				add("history", "line", "gpu.usage", "GPU HISTORY", 266, 20, 350, 106, 14, s.accent)
				add("cpu", "bar", "cpu.usage", "CPU", 22, 138, 594, 30, 14, s.accent)
			case 4:
				add("title", "text", "", "> SYSTEM ONLINE", 16, 12, 604, 24, 14, s.accent)
				add("cpu", "metric", "cpu.usage", "CPU //", 16, 48, 195, 70, 21, s.fg)
				add("gpu", "metric", "gpu.usage", "GPU //", 226, 48, 190, 70, 21, s.fg)
				add("ram", "metric", "ram.usage", "MEM //", 436, 48, 188, 70, 21, s.fg)
				add("history", "line", "cpu.usage", "", 16, 126, 608, 40, 7, s.accent)
			}
		} else {
			add("title", "text", "", strings.ToUpper(s.name)+" / SYSTEM MONITOR", 28, 22, 584, 32, 21, s.fg)
			switch i {
			case 0:
				add("cpu", "metric", "cpu.usage", "CPU LOAD", 28, 80, 270, 88, 35, s.accent)
				add("gpu", "metric", "gpu.temp", "GPU TEMP", 336, 80, 276, 88, 35, s.fg)
				add("history", "line", "cpu.usage", "PROCESSOR / 10 MIN", 28, 190, 584, 170, 14, s.accent)
				add("ram", "bar", "ram.usage", "MEMORY", 28, 390, 584, 52, 14, s.fg)
			case 1:
				add("cpu", "pie", "cpu.usage", "CPU LOAD", 28, 92, 258, 256, 21, s.accent)
				add("gpu", "metric", "gpu.temp", "GPU TEMP", 326, 106, 286, 94, 35, s.fg)
				add("disk", "metric", "disk.free", "DISK FREE", 326, 224, 286, 94, 28, s.accent)
				add("ram", "bar", "ram.usage", "MEMORY", 28, 393, 584, 54, 14, s.fg)
			case 2:
				add("cpu", "metric", "cpu.usage", "PROCESSOR", 28, 86, 280, 105, 42, s.fg)
				add("gpu", "metric", "gpu.usage", "GRAPHICS", 336, 86, 276, 105, 42, s.accent)
				add("ram", "metric", "ram.free", "MEMORY AVAILABLE", 28, 229, 280, 92, 28, s.accent)
				add("disk", "metric", "disk.free", "STORAGE AVAILABLE", 336, 229, 276, 92, 28, s.fg)
				add("storage", "bar", "disk.used", "SYSTEM DISK", 28, 372, 584, 68, 14, s.accent)
			case 3:
				add("ram", "metric", "ram.usage", "MEMORY", 28, 84, 584, 102, 49, s.fg)
				add("history", "line", "gpu.usage", "GRAPHICS HISTORY", 28, 218, 360, 222, 14, s.accent)
				add("cpu", "pie", "cpu.usage", "CPU", 414, 237, 198, 203, 14, s.fg)
			case 4:
				add("cpu", "metric", "cpu.usage", "01 / CPU LOAD", 28, 83, 276, 83, 28, s.fg)
				add("gpu", "metric", "gpu.usage", "02 / GPU LOAD", 336, 83, 276, 83, 28, s.fg)
				add("ram", "bar", "ram.usage", "03 / MEMORY", 28, 201, 584, 55, 14, s.accent)
				add("disk", "bar", "disk.used", "04 / STORAGE", 28, 280, 584, 55, 14, s.accent)
				add("history", "line", "cpu.usage", "> PROCESSOR TRACE", 28, 363, 584, 87, 14, s.accent)
			}
		}
		out = append(out, Theme{s.id, s.name, s.description, l})
	}
	return out
}
