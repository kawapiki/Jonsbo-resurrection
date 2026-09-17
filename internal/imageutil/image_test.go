package imageutil

import (
	"bytes"
	"image"
	"image/color"
	"testing"
)

func TestBGRConvertsNonzeroOriginAndRowOrder(t *testing.T) {
	im := image.NewNRGBA(image.Rect(10, 20, 12, 22))
	im.SetNRGBA(10, 20, color.NRGBA{255, 0, 0, 255})
	im.SetNRGBA(11, 20, color.NRGBA{0, 255, 0, 255})
	im.SetNRGBA(10, 21, color.NRGBA{0, 0, 255, 255})
	im.SetNRGBA(11, 21, color.NRGBA{1, 2, 3, 255})
	got := BGR(im)
	want := []byte{0, 0, 255, 0, 255, 0, 255, 0, 0, 3, 2, 1}
	if !bytes.Equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestFitPreservesAspectAndLetterboxes(t *testing.T) {
	src := image.NewRGBA(image.Rect(10, 20, 12, 21))
	src.Set(10, 20, color.White)
	src.Set(11, 20, color.White)
	dst, err := Fit(src, 4, 4)
	if err != nil {
		t.Fatal(err)
	}
	if dst.Bounds() != image.Rect(0, 0, 4, 4) {
		t.Fatal(dst.Bounds())
	}
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			r, _, _, _ := dst.At(x, y).RGBA()
			want := uint32(0)
			if y == 1 || y == 2 {
				want = 65535
			}
			if r != want {
				t.Fatalf("pixel %d,%d = %d", x, y, r)
			}
		}
	}
	if _, err := Fit(src, 0, 4); err == nil {
		t.Fatal("accepted zero width")
	}
}

func TestRotateClockwiseMapsLandscapeToNativePanel(t *testing.T) {
	src := image.NewRGBA(image.Rect(3, 5, 6, 7))
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			src.SetRGBA(x+3, y+5, color.RGBA{byte(1 + y*3 + x), 0, 0, 255})
		}
	}
	got, err := Rotate(src, 90)
	if err != nil {
		t.Fatal(err)
	}
	if got.Bounds() != image.Rect(0, 0, 2, 3) {
		t.Fatal(got.Bounds())
	}
	want := []byte{4, 1, 5, 2, 6, 3}
	i := 0
	for y := 0; y < 3; y++ {
		for x := 0; x < 2; x++ {
			r, _, _, _ := got.At(x, y).RGBA()
			if byte(r>>8) != want[i] {
				t.Fatalf("pixel %d,%d wrong", x, y)
			}
			i++
		}
	}
	if _, err := Rotate(src, 45); err == nil {
		t.Fatal("accepted non-right-angle rotation")
	}
}
