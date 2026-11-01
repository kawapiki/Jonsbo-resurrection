package configurator

import (
	"context"
	"image"
	"image/color"
	"math"
	"time"

	"github.com/kawapiki/Jonsbo-resurrection/pkg/module"
)

// Mascots use a transparent three-cell sheet: open eyes, blink, wink.
func layoutAssetIDs(l Layout) []string {
	ids := []string{}
	if l.Background.AssetID != "" {
		ids = append(ids, l.Background.AssetID)
	}
	for _, o := range l.Overlays {
		if o.AssetID != "" {
			ids = append(ids, o.AssetID)
		}
	}
	return ids
}

func validateLayoutAssets(l Layout, assets map[string]Asset) error {
	for _, id := range layoutAssetIDs(l) {
		if _, ok := assets[id]; !ok {
			return invalid("layout asset missing")
		}
	}
	for _, o := range l.Overlays {
		if o.AssetID == "" {
			continue
		}
		a := assets[o.AssetID]
		if a.FrameCount != 1 || a.Width != 3*a.Height {
			return invalid("mascot requires a three-cell sprite sheet")
		}
	}
	return nil
}

func ease(t float64) float64 { t = math.Max(0, math.Min(1, t)); return t * t * (3 - 2*t) }

func mascotPose(now time.Time, animate bool, farewell float64) (cell int, x, y, size, angle float64) {
	x, y, size = 120, 54, 82
	if animate {
		t := float64(now.UnixMilli()%24000) / 1000
		y += 1.3 * math.Sin(t*math.Pi/3)
		angle = .025 * math.Sin(t*math.Pi/4)
		// Uneven pauses, one double blink and a less frequent wink.
		for _, start := range []float64{2.8, 7.5, 7.95, 13.1, 21.3} {
			if t >= start && t < start+.25 {
				cell = 1
			}
		}
		if t >= 17.5 && t < 18.25 {
			cell = 2
		}
	}
	if farewell >= 0 {
		p := ease(farewell / .78)
		y += (120 - y) * p
		size += (168 - size) * p
		angle *= 1 - p
		if farewell >= .73 {
			cell = 2
		}
		if farewell >= .94 {
			cell = 1
		}
	}
	return
}

// Bilinear sampling preserves the sprite's premultiplied alpha, including its outline.
func drawMascot(dst *image.RGBA, sheet image.Image, now time.Time, animate bool, farewell float64) {
	cell, cx, cy, size, angle := mascotPose(now, animate, farewell)
	scale := math.Min(float64(dst.Bounds().Dx()), float64(dst.Bounds().Dy())) / 240
	cx = float64(dst.Bounds().Dx())/2 + (cx-120)*scale
	cy = float64(dst.Bounds().Dy())/2 + (cy-120)*scale
	size *= scale
	cellSize := sheet.Bounds().Dy()
	cs, sn := math.Cos(angle), math.Sin(angle)
	for y := max(0, int(cy-size*.75)); y < min(dst.Bounds().Dy(), int(math.Ceil(cy+size*.75))); y++ {
		for x := max(0, int(cx-size*.75)); x < min(dst.Bounds().Dx(), int(math.Ceil(cx+size*.75))); x++ {
			dx, dy := float64(x)+.5-cx, float64(y)+.5-cy
			sx, sy := (dx*cs+dy*sn)/size*float64(cellSize)+float64(cellSize)/2-.5, (-dx*sn+dy*cs)/size*float64(cellSize)+float64(cellSize)/2-.5
			ix, iy := int(math.Floor(sx)), int(math.Floor(sy))
			fx, fy := sx-float64(ix), sy-float64(iy)
			var rr, gg, bb, aa float64
			for j := 0; j < 2; j++ {
				for i := 0; i < 2; i++ {
					if ix+i < 0 || ix+i >= cellSize || iy+j < 0 || iy+j >= cellSize {
						continue
					}
					r, g, b, a := sheet.At(sheet.Bounds().Min.X+cell*cellSize+ix+i, sheet.Bounds().Min.Y+iy+j).RGBA()
					wx, wy := 1-fx, 1-fy
					if i == 1 {
						wx = fx
					}
					if j == 1 {
						wy = fy
					}
					weight := wx * wy / 257
					rr += float64(r) * weight
					gg += float64(g) * weight
					bb += float64(b) * weight
					aa += float64(a) * weight
				}
			}
			bg := dst.RGBAAt(x, y)
			inv := 1 - aa/255
			dst.SetRGBA(x, y, color.RGBA{uint8(math.Min(255, rr+float64(bg.R)*inv)), uint8(math.Min(255, gg+float64(bg.G)*inv)), uint8(math.Min(255, bb+float64(bg.B)*inv)), uint8(math.Min(255, aa+float64(bg.A)*inv))})
		}
	}
}

func (m *Module) drawThermal(ctx context.Context, dst *image.RGBA, o Overlay, value *float64, now time.Time, farewell float64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	thermalOrbitAt(dst, o, value, now, farewell)
	if o.AssetID == "" {
		return nil
	}
	m.mu.RLock()
	a, ok := m.assets[o.AssetID]
	m.mu.RUnlock()
	if !ok {
		return module.ErrNotFound
	}
	if a.FrameCount != 1 || a.Width != 3*a.Height {
		return invalid("mascot requires a three-cell sprite sheet")
	}
	sheet, err := m.assetFrame(a, 0)
	if err != nil {
		return err
	}
	drawMascot(dst, sheet, now, o.Animate == nil || *o.Animate, farewell)
	return nil
}

// FarewellFrame remains available after the sensor modules have stopped.
func (m *Module) FarewellFrame(ctx context.Context, view string, elapsed time.Duration) (image.Image, error) {
	m.mu.RLock()
	l, ok := m.layouts[view]
	m.mu.RUnlock()
	if !ok {
		return nil, module.ErrNotFound
	}
	enabled := false
	for _, o := range l.Overlays {
		if o.Type == "thermal-orbit" && o.AssetID != "" && o.Opacity > 0 {
			enabled = true
		}
	}
	if !enabled {
		return nil, module.ErrUnsupported
	}
	return m.renderTransition(ctx, l, time.Now(), math.Min(1, math.Max(0, float64(elapsed)/float64(1500*time.Millisecond))))
}
