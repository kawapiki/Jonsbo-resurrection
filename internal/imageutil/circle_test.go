package imageutil

import (
	"image"
	"image/color"
	"testing"
)

func TestRingProgressAndTransparentInterior(t *testing.T) {
	fg := color.RGBA{100, 200, 240, 255}
	bg := color.RGBA{20, 40, 48, 255}
	for _, origin := range []image.Point{{}, {13, 17}} {
		im := image.NewRGBA(image.Rectangle{Min: origin, Max: origin.Add(image.Pt(20, 20))})
		Ring(im, 9, 2, .25, fg, bg)
		for _, p := range []struct {
			x, y int
			want color.RGBA
		}{{10, 1, fg}, {18, 9, fg}, {18, 10, bg}, {1, 10, bg}, {10, 10, color.RGBA{}}, {0, 0, color.RGBA{}}} {
			if got := im.RGBAAt(origin.X+p.x, origin.Y+p.y); got != p.want {
				t.Fatalf("origin=%v pixel=(%d,%d): got %v want %v", origin, p.x, p.y, got, p.want)
			}
		}
	}
}
func TestRingEmptyAndTinyBounds(t *testing.T) {
	fg, bg := color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}
	im := image.NewRGBA(image.Rect(0, 0, 20, 20))
	Ring(im, 9, 2, 0, fg, bg)
	if got := im.RGBAAt(10, 1); got != bg {
		t.Fatal("zero or missing reading must show track", got)
	}
	for _, r := range []float64{-1, 0} {
		tiny := image.NewRGBA(image.Rect(0, 0, 1, 1))
		Ring(tiny, r, 3, 1, fg, bg)
		if tiny.RGBAAt(0, 0).A != 0 {
			t.Fatal("nonpositive radius must be empty")
		}
	}
}
func TestMaskCirclePreservesInterior(t *testing.T) {
	im := image.NewRGBA(image.Rect(4, 6, 24, 26))
	c := color.RGBA{R: 123, A: 255}
	for y := 6; y < 26; y++ {
		for x := 4; x < 24; x++ {
			im.SetRGBA(x, y, c)
		}
	}
	MaskCircle(im)
	if got := im.RGBAAt(4, 6); got != (color.RGBA{A: 255}) {
		t.Fatal("outside must be opaque black", got)
	}
	if got := im.RGBAAt(14, 16); got != c {
		t.Fatal("interior changed", got)
	}
}
