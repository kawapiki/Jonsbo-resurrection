package configurator

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/kawapiki/Jonsbo-resurrection/pkg/module"
)

func frameIndex(a Asset, now time.Time) int {
	if a.FrameCount <= 1 {
		return 0
	}
	tick := now.Unix()*int64(a.FPS) + int64(now.Nanosecond())*int64(a.FPS)/int64(time.Second)
	return int((tick%int64(a.FrameCount) + int64(a.FrameCount)) % int64(a.FrameCount))
}
func (m *Module) assetFrame(a Asset, index int) (image.Image, error) {
	if !idPattern.MatchString(a.ID) || index < 0 || index >= a.FrameCount {
		return nil, invalid("asset frame")
	}
	b, e := readBounded(m.framePath(a.ID, index), maxAssetBytes)
	if e != nil {
		return nil, e
	}
	cfg, _, e := image.DecodeConfig(bytes.NewReader(b))
	if e != nil || cfg.Width != a.Width || cfg.Height != a.Height {
		return nil, invalid("stored image dimensions")
	}
	im, _, e := image.Decode(bytes.NewReader(b))
	if e != nil {
		return nil, invalid("stored image corrupt")
	}
	return im, nil
}
func rgba(s string, alpha float64) color.RGBA {
	v, _ := strconv.ParseUint(strings.TrimPrefix(s, "#"), 16, 32)
	a := uint8(math.Round(alpha * 255))
	return color.RGBA{uint8(uint32(uint8(v>>16)) * uint32(a) / 255), uint8(uint32(uint8(v>>8)) * uint32(a) / 255), uint8(uint32(uint8(v)) * uint32(a) / 255), a}
}
func fill(dst *image.RGBA, r image.Rectangle, c color.RGBA) {
	draw.Draw(dst, r, &image.Uniform{C: c}, image.Point{}, draw.Over)
}
func (m *Module) render(ctx context.Context, l Layout, now time.Time) (image.Image, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if e := ValidateLayout(l); e != nil {
		return nil, e
	}
	m.mu.RLock()
	a, hasAsset := m.assets[l.Background.AssetID]
	metrics := append([]Metric(nil), m.metrics...)
	history := append([]History(nil), m.history...)
	m.mu.RUnlock()
	out := image.NewRGBA(image.Rect(0, 0, l.Width, l.Height))
	fill(out, out.Bounds(), rgba(l.Background.Color, 1))
	if l.Background.AssetID != "" {
		if !hasAsset {
			return nil, fmt.Errorf("%w: background asset", module.ErrNotFound)
		}
		im, e := m.assetFrame(a, frameIndex(a, now))
		if e != nil {
			return nil, e
		}
		// Center-crop the background to fill either device shape without stretching.
		scale := math.Max(float64(l.Width)/float64(im.Bounds().Dx()), float64(l.Height)/float64(im.Bounds().Dy()))
		ox := (float64(im.Bounds().Dx()) - float64(l.Width)/scale) / 2
		oy := (float64(im.Bounds().Dy()) - float64(l.Height)/scale) / 2
		for y := 0; y < l.Height; y++ {
			if y%32 == 0 {
				if e := ctx.Err(); e != nil {
					return nil, e
				}
			}
			for x := 0; x < l.Width; x++ {
				sx := min(im.Bounds().Dx()-1, int(float64(x)/scale+ox))
				sy := min(im.Bounds().Dy()-1, int(float64(y)/scale+oy))
				r, g, b, al := im.At(sx, sy).RGBA()
				alpha := float64(al) / 65535 * l.Background.Opacity
				bg := out.RGBAAt(x, y)
				out.SetRGBA(x, y, color.RGBA{uint8(float64(r>>8)*l.Background.Opacity + float64(bg.R)*(1-alpha)), uint8(float64(g>>8)*l.Background.Opacity + float64(bg.G)*(1-alpha)), uint8(float64(b>>8)*l.Background.Opacity + float64(bg.B)*(1-alpha)), 255})
			}
		}
	}
	lookup := map[string]Metric{}
	for _, v := range metrics {
		lookup[v.ID] = v
	}
	for _, o := range l.Overlays {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		if o.Opacity == 0 {
			continue
		}
		layer := image.NewRGBA(image.Rect(0, 0, o.W, o.H))
		fg := rgba(o.Color, 1)
		metric := lookup[o.Metric]
		label := o.Label
		if label == "" && o.Type != "text" {
			label = metric.Label
		}
		if hasAsset {
			fill(layer, layer.Bounds(), rgba(l.Background.Color, .78))
		}
		labelSize := min(14, o.FontSize)
		labelH := max(9, (labelSize/7)*7+4)
		value := "--"
		if metric.Value != nil {
			value = fmt.Sprintf("%.0f", *metric.Value)
			if metric.Unit == "GiB" {
				value = fmt.Sprintf("%.1f", *metric.Value)
			}
			value += " " + metric.Unit
		}
		ratio := 0.0
		if metric.Value != nil {
			ratio = max(0, math.Min(1, (*metric.Value-o.Min)/(o.Max-o.Min)))
		}
		switch o.Type {
		case "text":
			text(layer, 0, 0, o.Label, o.FontSize, fg)
		case "metric":
			text(layer, 0, 0, label, labelSize, fg)
			text(layer, 0, labelH+3, value, o.FontSize, fg)
		case "bar":
			text(layer, 0, 0, label+"  "+value, labelSize, fg)
			top := min(o.H-1, labelH+3)
			track := fg
			track.R /= 5
			track.G /= 5
			track.B /= 5
			fill(layer, image.Rect(0, top, o.W, o.H), track)
			if metric.Value != nil {
				fill(layer, image.Rect(0, top, int(float64(o.W)*ratio), o.H), fg)
			}
		case "pie":
			text(layer, 0, 0, label, labelSize, fg)
			cy := float64(labelH) + float64(o.H-labelH)/2
			cx := float64(o.W) / 2
			radius := math.Min(float64(o.W)/2, float64(o.H-labelH)/2) - 2
			inner := radius * .64
			for y := labelH; y < o.H; y++ {
				for x := 0; x < o.W; x++ {
					dx, dy := float64(x)-cx, float64(y)-cy
					distance := dx*dx + dy*dy
					if distance > radius*radius || distance < inner*inner {
						continue
					}
					angle := math.Atan2(dy, dx) + math.Pi/2
					if angle < 0 {
						angle += 2 * math.Pi
					}
					c := fg
					if metric.Value == nil || angle > ratio*2*math.Pi {
						c.R /= 5
						c.G /= 5
						c.B /= 5
					}
					layer.SetRGBA(x, y, c)
				}
			}
			display := "--"
			if metric.Value != nil {
				display = fmt.Sprintf("%.0f", *metric.Value)
			}
			size := min(o.FontSize, 21)
			tw := textWidth(display, size)
			text(layer, max(0, (o.W-tw)/2), int(cy)-size/2, display, size, fg)
		case "line":
			text(layer, 0, 0, label, labelSize, fg)
			top := min(o.H-1, labelH+4)
			bottom := o.H - 2
			grid := fg
			grid.R /= 5
			grid.G /= 5
			grid.B /= 5
			for j := 0; j < 4; j++ {
				y := top + (bottom-top)*j/3
				fill(layer, image.Rect(0, y, o.W, y+1), grid)
			}
			hasPoint := false
			px, py := 0, 0
			var prevTime time.Time
			cutoff := now.Add(-10 * time.Minute)
			for _, point := range history {
				if point.Time.Before(cutoff) || point.Time.After(now) {
					continue
				}
				v := point.Values[o.Metric]
				if v == nil {
					hasPoint = false
					continue
				}
				x := int(float64(o.W-1) * point.Time.Sub(cutoff).Seconds() / 600)
				r := max(0, math.Min(1, (*v-o.Min)/(o.Max-o.Min)))
				y := bottom - int(float64(max(0, bottom-top))*r)
				if hasPoint && point.Time.Sub(prevTime) <= 3*time.Second {
					line(layer, px, py, x, y, fg)
				} else {
					fill(layer, image.Rect(x, y, x+2, y+2), fg)
				}
				px, py, prevTime, hasPoint = x, y, point.Time, true
			}
			if !hasPoint {
				text(layer, 5, top+4, "WAITING FOR DATA", 7, fg)
			}
		}
		mask := image.NewUniform(color.Alpha{A: uint8(math.Round(o.Opacity * 255))})
		draw.DrawMask(out, image.Rect(o.X, o.Y, o.X+o.W, o.Y+o.H), layer, image.Point{}, mask, image.Point{}, draw.Over)
	}
	return out, nil
}
func line(im *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	dx := abs(x1 - x0)
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	dy := -abs(y1 - y0)
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		fill(im, image.Rect(x0, y0, x0+2, y0+2), c)
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}
func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
func textWidth(s string, size int) int { return len([]rune(s)) * 6 * max(1, size/7) }

// A crisp 5x7 bitmap font is scaled by whole pixels for small physical LCDs.
// Unsupported glyphs are displayed as '?' rather than silently disappearing.
func text(im *image.RGBA, x, y int, s string, size int, c color.RGBA) {
	scale := max(1, size/7)
	start := x
	for _, r := range strings.ToUpper(s) {
		if r == '\n' {
			y += 8 * scale
			x = start
			continue
		}
		glyph, ok := glyphs[r]
		if !ok {
			glyph = glyphs['?']
		}
		if x+5*scale > im.Bounds().Dx() {
			break
		}
		for row, bits := range glyph {
			for col := 0; col < 5; col++ {
				if bits&(1<<uint(4-col)) != 0 {
					fill(im, image.Rect(x+col*scale, y+row*scale, x+(col+1)*scale, y+(row+1)*scale), c)
				}
			}
		}
		x += 6 * scale
	}
}

var glyphs = map[rune][7]uint8{
	' ': {0, 0, 0, 0, 0, 0, 0}, 'A': {14, 17, 17, 31, 17, 17, 17}, 'B': {30, 17, 17, 30, 17, 17, 30}, 'C': {14, 17, 16, 16, 16, 17, 14}, 'D': {30, 17, 17, 17, 17, 17, 30}, 'E': {31, 16, 16, 30, 16, 16, 31}, 'F': {31, 16, 16, 30, 16, 16, 16}, 'G': {14, 17, 16, 23, 17, 17, 15}, 'H': {17, 17, 17, 31, 17, 17, 17}, 'I': {14, 4, 4, 4, 4, 4, 14}, 'J': {7, 2, 2, 2, 18, 18, 12}, 'K': {17, 18, 20, 24, 20, 18, 17}, 'L': {16, 16, 16, 16, 16, 16, 31}, 'M': {17, 27, 21, 21, 17, 17, 17}, 'N': {17, 25, 21, 19, 17, 17, 17}, 'O': {14, 17, 17, 17, 17, 17, 14}, 'P': {30, 17, 17, 30, 16, 16, 16}, 'Q': {14, 17, 17, 17, 21, 18, 13}, 'R': {30, 17, 17, 30, 20, 18, 17}, 'S': {15, 16, 16, 14, 1, 1, 30}, 'T': {31, 4, 4, 4, 4, 4, 4}, 'U': {17, 17, 17, 17, 17, 17, 14}, 'V': {17, 17, 17, 17, 17, 10, 4}, 'W': {17, 17, 17, 21, 21, 21, 10}, 'X': {17, 17, 10, 4, 10, 17, 17}, 'Y': {17, 17, 10, 4, 4, 4, 4}, 'Z': {31, 1, 2, 4, 8, 16, 31},
	'0': {14, 17, 19, 21, 25, 17, 14}, '1': {4, 12, 4, 4, 4, 4, 14}, '2': {14, 17, 1, 2, 4, 8, 31}, '3': {30, 1, 1, 14, 1, 1, 30}, '4': {2, 6, 10, 18, 31, 2, 2}, '5': {31, 16, 16, 30, 1, 1, 30}, '6': {14, 16, 16, 30, 17, 17, 14}, '7': {31, 1, 2, 4, 8, 8, 8}, '8': {14, 17, 17, 14, 17, 17, 14}, '9': {14, 17, 17, 15, 1, 1, 14},
	'.': {0, 0, 0, 0, 0, 6, 6}, ',': {0, 0, 0, 0, 6, 6, 4}, ':': {0, 6, 6, 0, 6, 6, 0}, '-': {0, 0, 0, 31, 0, 0, 0}, '_': {0, 0, 0, 0, 0, 0, 31}, '/': {1, 1, 2, 4, 8, 16, 16}, '%': {25, 25, 2, 4, 8, 19, 19}, '?': {14, 17, 1, 2, 4, 0, 4}, '>': {16, 8, 4, 2, 4, 8, 16}, '<': {1, 2, 4, 8, 4, 2, 1}, '+': {0, 4, 4, 31, 4, 4, 0}, '=': {0, 0, 31, 0, 31, 0, 0}, '(': {2, 4, 8, 8, 8, 4, 2}, ')': {8, 4, 2, 2, 2, 4, 8}, '!': {4, 4, 4, 4, 4, 0, 4}, '°': {6, 9, 9, 6, 0, 0, 0},
}
