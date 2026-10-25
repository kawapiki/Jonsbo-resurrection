package configurator

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"testing"
	"time"
)

func TestCompactNumberFitsBelowItsLabel(t *testing.T) {
	m := fresh(t)
	v := 25.0
	m.metrics = []Metric{{ID: "cpu.usage", Unit: "%", Value: &v}}
	l := Layout{ID: "number-fit", Name: "fit", Width: 240, Height: 240, Background: Background{Color: "#000000", Opacity: 1}, Overlays: []Overlay{{ID: "n", Type: "number", Metric: "cpu.usage", Label: "CPU", X: 45, Y: 50, W: 150, H: 70, FontSize: 56, Color: "#62DCE5", Max: 100, Opacity: 1}}}
	big, e := m.render(context.Background(), l, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	l.Overlays[0].FontSize = 49
	fit, e := m.render(context.Background(), l, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(big.(*image.RGBA).Pix, fit.(*image.RGBA).Pix) {
		t.Fatal("large number clipped instead of fitting below 14px label")
	}
}

func TestCompactWidgetsRenderPersistAndKeepRingCenterEmpty(t *testing.T) {
	m := fresh(t)
	for _, typ := range []string{"ring", "number", "sparkline"} {
		l := Layout{ID: "compact-" + typ, Name: typ, Width: 240, Height: 240, Background: Background{Color: "#000000", Opacity: 1}, Overlays: []Overlay{{ID: "part", Type: typ, Metric: "cpu.usage", X: 10, Y: 10, W: 220, H: 220, FontSize: 28, Color: "#62DCE5", Max: 100, Opacity: 1}}}
		if w := request(t, m, &fakeDisplays{}, "PUT", "/layouts/"+l.ID, l); w.Code != 200 {
			t.Fatalf("%s: %s", typ, w.Body.String())
		}
		im, e := m.render(context.Background(), l, time.Now())
		if e != nil {
			t.Fatal(e)
		}
		if typ == "ring" && im.At(120, 120) != (color.RGBA{0, 0, 0, 255}) {
			t.Fatal("ring covers center content")
		}
	}
	reopened, e := New(m.dir, &fakeSource{})
	if e != nil {
		t.Fatal(e)
	}
	if e = reopened.ValidateView("compact-ring"); e != nil {
		t.Fatal(e)
	}
}

func TestSparklineDoesNotJoinMissingSamples(t *testing.T) {
	now := time.Now()
	v := 50.0
	o := Overlay{W: 120, H: 30, Metric: "cpu.usage", Max: 100}
	m := fresh(t)
	m.history = []History{{Time: now.Add(-120 * time.Second), Values: map[string]*float64{"cpu.usage": &v}}, {Time: now.Add(-119 * time.Second), Values: map[string]*float64{"cpu.usage": nil}}, {Time: now, Values: map[string]*float64{"cpu.usage": &v}}}
	o.X = 60
	o.Y = 100
	o.ID = "trace"
	o.Type = "sparkline"
	o.FontSize = 7
	o.Color = "#FFFFFF"
	o.Opacity = 1
	im, e := m.render(context.Background(), Layout{ID: "trace", Name: "trace", Width: 240, Height: 240, Background: Background{Color: "#000000", Opacity: 1}, Overlays: []Overlay{o}}, now)
	if e != nil {
		t.Fatal(e)
	}
	if im.At(120, 114) != (color.RGBA{0, 0, 0, 255}) {
		t.Fatal("graph connected missing samples")
	}
}
