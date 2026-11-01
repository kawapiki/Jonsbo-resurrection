package configurator

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"time"
)

// Rasterized from IBM Plex Sans SemiBold (SIL OFL), with slashed zero.
//
//go:embed fonts/plex-dial.png
var dialFontPNG []byte

//go:embed fonts/plex-dial.json
var dialFontJSON []byte

type dialGlyph struct{ X, W int }

var dialAtlas, dialGlyphs = loadDialFont()

func loadDialFont() (image.Image, map[string]dialGlyph) {
	im, err := png.Decode(bytes.NewReader(dialFontPNG))
	if err != nil {
		panic(err)
	}
	var glyphs map[string]dialGlyph
	if err := json.Unmarshal(dialFontJSON, &glyphs); err != nil {
		panic(err)
	}
	return im, glyphs
}

func dialWidth(s string, size float64) float64 {
	w := 0.0
	for _, c := range s {
		w += float64(dialGlyphs[string(c)].W) * size / 96
	}
	return w
}

// dialText centers the visible glyphs, with bilinear sampling of the alpha atlas.
func dialText(dst *image.RGBA, s string, cy, size, maxWidth float64, ink color.RGBA) {
	if w := dialWidth(s, size); w > maxWidth {
		size *= maxWidth / w
	}
	x := (float64(dst.Bounds().Dx()) - dialWidth(s, size)) / 2
	y := cy - size/2
	scale := size / 96
	for _, c := range s {
		g := dialGlyphs[string(c)]
		for py := int(math.Floor(y)); py < int(math.Ceil(y+112*scale)); py++ {
			for px := int(math.Floor(x)); px < int(math.Ceil(x+float64(g.W)*scale)); px++ {
				if !image.Pt(px, py).In(dst.Bounds()) {
					continue
				}
				sx, sy := (float64(px)+.5-x)/scale-.5, (float64(py)+.5-y)/scale-.5
				ix, iy := int(math.Floor(sx)), int(math.Floor(sy))
				fx, fy := sx-float64(ix), sy-float64(iy)
				alpha := 0.0
				for j := 0; j < 2; j++ {
					for i := 0; i < 2; i++ {
						if ix+i < 0 || ix+i >= g.W || iy+j < 0 || iy+j >= 112 {
							continue
						}
						v, _, _, _ := dialAtlas.At(g.X+ix+i, iy+j).RGBA()
						wx, wy := 1-fx, 1-fy
						if i == 1 {
							wx = fx
						}
						if j == 1 {
							wy = fy
						}
						alpha += float64(v) / 65535 * wx * wy
					}
				}
				bg := dst.RGBAAt(px, py)
				dst.SetRGBA(px, py, color.RGBA{
					uint8(float64(ink.R)*alpha + float64(bg.R)*(1-alpha)),
					uint8(float64(ink.G)*alpha + float64(bg.G)*(1-alpha)),
					uint8(float64(ink.B)*alpha + float64(bg.B)*(1-alpha)),
					uint8(255*alpha + float64(bg.A)*(1-alpha)),
				})
			}
		}
		x += float64(g.W) * scale
	}
}

func thermalValue(value *float64) string {
	if value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) {
		return "--"
	}
	return fmt.Sprintf("%.1f", *value)
}

// thermalOrbit keeps all motion at the rim, leaving a quiet, legible center.
// The transparent upper center leaves room for a personal background illustration.
func thermalOrbit(dst *image.RGBA, o Overlay, value *float64, now time.Time) {
	thermalOrbitAt(dst, o, value, now, -1)
}

func thermalOrbitAt(dst *image.RGBA, o Overlay, value *float64, now time.Time, farewell float64) {
	w, h := float64(dst.Bounds().Dx()), float64(dst.Bounds().Dy())
	scale := math.Min(w, h) / 240
	cx, cy := w/2, h/2
	phase := 0.0
	if o.Animate == nil || *o.Animate {
		phase = float64(now.UnixMilli()%8000) / 8000 * 2 * math.Pi
	}
	ink := rgba(o.Color, 1)
	for y := 0; y < int(h); y++ {
		for x := 0; x < int(w); x++ {
			dx, dy := (float64(x)+.5-cx)/scale, (float64(y)+.5-cy)/scale
			r := math.Hypot(dx, dy)
			if r < 96 || r > 120 {
				continue
			}
			a := math.Atan2(dy, dx) - phase
			// Two soft comets orbit in eight seconds; a fine inner ring counter-rotates.
			peak := math.Pow((1+math.Cos(a))/2, 18) + .65*math.Pow((1+math.Cos(a+math.Pi))/2, 24)
			core := math.Exp(-math.Pow((r-110)/1.05, 2))
			glow := math.Exp(-math.Pow((r-110)/4.3, 2))
			inner := math.Exp(-math.Pow((r-103)/.65, 2)) * (.1 + .3*math.Pow((1+math.Cos(a+2*phase+1.4))/2, 8))
			alpha := math.Min(1, core*(.28+.72*peak)+glow*(.06+.3*peak)+inner)
			white := math.Min(.8, peak*core*.7)
			dst.SetRGBA(x, y, color.RGBA{
				uint8((float64(ink.R)*(1-white) + 255*white) * alpha),
				uint8((float64(ink.G)*(1-white) + 240*white) * alpha),
				uint8((float64(ink.B)*(1-white) + 255*white) * alpha), uint8(255 * alpha)})
		}
	}
	alpha := 1.0
	if farewell >= 0 {
		alpha = 1 - ease(farewell/.28)
	}
	if alpha > 0 {
		reading := image.NewRGBA(dst.Bounds())
		dialText(reading, "CPU", cy-26*scale, 17*scale, 180*scale, color.RGBA{203, 172, 237, 255})
		dialText(reading, thermalValue(value), cy+22*scale, float64(o.FontSize)*scale, 186*scale, color.RGBA{249, 242, 255, 255})
		dialText(reading, "°C", cy+74*scale, 23*scale, 100*scale, color.RGBA{211, 179, 244, 255})
		draw.DrawMask(dst, dst.Bounds(), reading, image.Point{}, image.NewUniform(color.Alpha{A: uint8(255 * alpha)}), image.Point{}, draw.Over)
	}
}
