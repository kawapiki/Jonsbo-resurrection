package imageutil

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
)

// Fit scales to fit, with black letterboxing and alpha composited on black.
func Fit(src image.Image, width, height int) (*image.RGBA, error) {
	if width <= 0 || height <= 0 || width > 4096 || height > 4096 || src.Bounds().Empty() {
		return nil, fmt.Errorf("invalid image dimensions")
	}
	b := src.Bounds()
	scale := math.Min(float64(width)/float64(b.Dx()), float64(height)/float64(b.Dy()))
	w := max(1, int(float64(b.Dx())*scale))
	h := max(1, int(float64(b.Dy())*scale))
	x0, y0 := (width-w)/2, (height-h)/2
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.Black), image.Point{}, draw.Src)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, bv, _ := src.At(b.Min.X+x*b.Dx()/w, b.Min.Y+y*b.Dy()/h).RGBA()
			dst.SetRGBA(x0+x, y0+y, color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(bv >> 8), 255})
		}
	}
	return dst, nil
}

func BGR(src image.Image) []byte {
	b := src.Bounds()
	out := make([]byte, 0, b.Dx()*b.Dy()*3)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, b, _ := src.At(x, y).RGBA()
			out = append(out, byte(b>>8), byte(g>>8), byte(r>>8))
		}
	}
	return out
}

// Rotate transforms a logical image into the panel's native memory orientation.
func Rotate(src image.Image, degrees int) (image.Image, error) {
	if degrees != 0 && degrees != 90 && degrees != 180 && degrees != 270 {
		return nil, fmt.Errorf("rotation must be 0, 90, 180 or 270")
	}
	if degrees == 0 {
		return src, nil
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dw, dh := w, h
	if degrees == 90 || degrees == 270 {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var dx, dy int
			switch degrees {
			case 90:
				dx, dy = h-1-y, x
			case 180:
				dx, dy = w-1-x, h-1-y
			case 270:
				dx, dy = y, w-1-x
			}
			dst.Set(dx, dy, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst, nil
}

// Pattern uses primary-color stripes and a large number to identify physical screens.
func Pattern(width, height, number int) *image.RGBA {
	im := image.NewRGBA(image.Rect(0, 0, width, height))
	colors := []color.RGBA{{220, 25, 40, 255}, {25, 200, 70, 255}, {30, 80, 230, 255}}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			c := colors[min(2, x*3/width)]
			if y < height/16 {
				c = color.RGBA{255, 255, 255, 255}
			}
			if y >= height-height/16 {
				c = color.RGBA{0, 0, 0, 255}
			}
			im.SetRGBA(x, y, c)
		}
	}
	digits := map[int][]string{1: {"010", "110", "010", "010", "111"}, 2: {"111", "001", "111", "100", "111"}, 3: {"111", "001", "111", "001", "111"}, 4: {"101", "101", "111", "001", "001"}}
	shape, ok := digits[number]
	if !ok {
		shape = digits[1]
	}
	size := min(width/5, height/8)
	x0, y0 := (width-3*size)/2, (height-5*size)/2
	for y, row := range shape {
		for x, v := range row {
			if v == '1' {
				draw.Draw(im, image.Rect(x0+x*size, y0+y*size, x0+(x+1)*size, y0+(y+1)*size), image.White, image.Point{}, draw.Src)
			}
		}
	}
	return im
}
