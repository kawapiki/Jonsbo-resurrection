package configurator

import (
	"bytes"
	"image"
	"math"
	"testing"
	"time"
)

func thermalOverlay() Overlay {
	return Overlay{ID: "cpu", Type: "thermal-orbit", Metric: "cpu.temp", Label: "CPU", W: 240, H: 240, FontSize: 68, Color: "#BF79FF", Min: 0, Max: 100, Opacity: 1}
}

func TestThermalAnimationKeepsReadingStill(t *testing.T) {
	o := thermalOverlay()
	v := 62.4
	a, b := image.NewRGBA(image.Rect(0, 0, 240, 240)), image.NewRGBA(image.Rect(0, 0, 240, 240))
	thermalOrbit(a, o, &v, time.Unix(0, 0))
	thermalOrbit(b, o, &v, time.Unix(2, 0))
	if bytes.Equal(a.Pix, b.Pix) {
		t.Fatal("animated ring did not move")
	}
	partialAlpha := false
	for y := 85; y < 205; y++ {
		for x := 30; x < 210; x++ {
			if math.Hypot(float64(x)-120, float64(y)-120) >= 95 {
				continue
			}
			if a.RGBAAt(x, y) != b.RGBAAt(x, y) {
				t.Fatal("animation moved central reading")
			}
			alpha := a.RGBAAt(x, y).A
			if alpha > 0 && alpha < 255 {
				partialAlpha = true
			}
		}
	}
	if !partialAlpha {
		t.Fatal("text edges are not antialiased")
	}
	if a.RGBAAt(120, 50).A != 0 {
		t.Fatal("character area must remain transparent")
	}
	animate := false
	o.Animate = &animate
	// Use fresh layers, as text is composited over existing pixels.
	a, b = image.NewRGBA(a.Bounds()), image.NewRGBA(b.Bounds())
	thermalOrbit(a, o, &v, time.Unix(0, 0))
	thermalOrbit(b, o, &v, time.Unix(2, 0))
	if !bytes.Equal(a.Pix, b.Pix) {
		t.Fatal("disabled animation still moves")
	}
}

func TestThermalMissingReading(t *testing.T) {
	if thermalValue(nil) != "--" {
		t.Fatal("missing sensor must not look like zero degrees")
	}
	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if thermalValue(&v) != "--" {
			t.Fatal("invalid reading shown")
		}
	}
	z := 0.0
	if thermalValue(&z) != "0.0" {
		t.Fatal("real zero lost")
	}
	m := fresh(t)
	o := thermalOverlay()
	w := request(t, m, nil, "POST", "/thermal-widget.png", o)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	o.Metric = "gpu.temp"
	w = request(t, m, nil, "POST", "/thermal-widget.png", o)
	if w.Code != 400 {
		t.Fatal("CPU-only widget accepted another metric", w.Code)
	}
}
