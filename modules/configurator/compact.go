package configurator

import (
	"github.com/kawapiki/Jonsbo-resurrection/internal/imageutil"
	"image"
	"image/color"
	"math"
	"strings"
	"time"
)

// Ring-only components leave their interior transparent for other widgets.
func compactRing(im *image.RGBA, ratio float64, available bool, fg color.RGBA) {
	radius := math.Max(0, float64(min(im.Bounds().Dx(), im.Bounds().Dy()))/2-1)
	track := color.RGBA{R: fg.R / 6, G: fg.G / 6, B: fg.B / 6, A: fg.A}
	if !available {
		ratio = 0
	}
	imageutil.Ring(im, radius, math.Max(3, radius*.045), ratio, fg, track)
}

func compactNumber(im *image.RGBA, o Overlay, label, value string, fg color.RGBA) {
	size := o.FontSize
	value = strings.ReplaceAll(value, " ", "")
	labelSize := 7
	if o.H >= 65 && o.W >= 120 {
		labelSize = 14
	}
	available := o.H - labelSize - 6
	for size > 7 && (textWidth(value, size) > o.W-4 || max(1, size/7)*7 > available) {
		size--
	}
	centeredText(im, 0, label, labelSize, max(1, o.W-4), fg)
	if available < 7 {
		return
	}
	centeredText(im, max(1, labelSize/7)*7+6, value, size, max(1, o.W-4), rgba("#E7EFF0", 1))
}

// Two minutes of history, with actual time spacing and breaks at missing data.
func compactSparkline(im *image.RGBA, o Overlay, history []History, now time.Time, fg color.RGBA) {
	cutoff := now.Add(-120 * time.Second)
	pen := false
	px, py := 0, 0
	var previous time.Time
	grid := fg
	grid.R /= 7
	grid.G /= 7
	grid.B /= 7
	fill(im, image.Rect(0, o.H-1, o.W, o.H), grid)
	for _, p := range history {
		if p.Time.Before(cutoff) || p.Time.After(now) {
			continue
		}
		v := p.Values[o.Metric]
		if v == nil || !finite(*v) {
			pen = false
			continue
		}
		x := int(float64(o.W-1) * p.Time.Sub(cutoff).Seconds() / 120)
		ratio := math.Max(0, math.Min(1, (*v-o.Min)/(o.Max-o.Min)))
		y := max(0, o.H-3) - int(float64(max(0, o.H-4))*ratio)
		if pen && p.Time.Sub(previous) > 0 && p.Time.Sub(previous) <= 3*time.Second {
			line(im, px, py, x, y, fg)
		} else {
			fill(im, image.Rect(x, y, x+2, y+2), fg)
		}
		px, py, previous, pen = x, y, p.Time, true
	}
	if !pen && o.W >= 42 && o.H >= 9 {
		centeredText(im, max(0, (o.H-7)/2), "--", 7, o.W, fg)
	}
}
