package imageutil

import (
	"image"
	"image/color"
	"math"
)

// Ring draws clockwise from twelve o'clock, leaving its interior transparent.
// A zero ratio draws only the track, including unavailable readings.
func Ring(im *image.RGBA, radius, thickness, ratio float64, fg, track color.RGBA) {
	if radius <= 0 || thickness <= 0 {
		return
	}
	b := im.Bounds()
	cx, cy := float64(b.Min.X+b.Max.X)/2, float64(b.Min.Y+b.Max.Y)/2
	inner := math.Max(0, radius-thickness)
	outer2, inner2 := radius*radius, inner*inner
	limit := ratio * 2 * math.Pi
	for y := b.Min.Y; y < b.Max.Y; y++ {
		dy := float64(y) + .5 - cy
		for x := b.Min.X; x < b.Max.X; x++ {
			dx := float64(x) + .5 - cx
			distance := dx*dx + dy*dy
			if distance > outer2 || distance < inner2 {
				continue
			}
			angle := math.Atan2(dy, dx) + math.Pi/2
			if angle < 0 {
				angle += 2 * math.Pi
			}
			c := track
			if angle < limit {
				c = fg
			}
			im.SetRGBA(x, y, c)
		}
	}
}

// MaskCircle blacks out pixels beyond the centered inscribed circle.
func MaskCircle(im *image.RGBA) {
	b := im.Bounds()
	cx, cy := float64(b.Min.X+b.Max.X)/2, float64(b.Min.Y+b.Max.Y)/2
	radius := float64(min(b.Dx(), b.Dy())) / 2
	for y := b.Min.Y; y < b.Max.Y; y++ {
		dy := float64(y) + .5 - cy
		for x := b.Min.X; x < b.Max.X; x++ {
			dx := float64(x) + .5 - cx
			if dx*dx+dy*dy > radius*radius {
				im.SetRGBA(x, y, color.RGBA{A: 255})
			}
		}
	}
}
