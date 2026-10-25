package dashboard

import (
	"github.com/kawapiki/Jonsbo-resurrection/internal/imageutil"
	"github.com/kawapiki/Jonsbo-resurrection/internal/telemetry"
	"image"
)

func renderCyclops(s telemetry.Snapshot) (*image.RGBA, error) {
	c, err := newCanvas(240, 240)
	if err != nil {
		return nil, err
	}
	defer c.close()
	c.rect(0, 0, 240, 240, background)
	c.text(94, 35, 18, cyan, "CPU")
	c.text(70, 58, 34, foreground, metric(s.CPU.TemperatureC, " C"))
	c.rect(50, 118, 140, 1, track)
	var gpu *float64
	if len(s.GPUs) > 0 {
		gpu = s.GPUs[0].TemperatureC
	}
	c.text(94, 132, 18, orange, "GPU")
	c.text(70, 155, 34, foreground, metric(gpu, " C"))
	im, err := c.image()
	if err != nil {
		return nil, err
	}
	imageutil.MaskCircle(im)
	return im, nil
}
