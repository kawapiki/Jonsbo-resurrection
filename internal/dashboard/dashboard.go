package dashboard

import (
	"fmt"
	"image"
	"github.com/kawapiki/Jonsbo-resurrection/internal/telemetry"
	"math"
)

func metric(v *float64, unit string) string {
	if v == nil || math.IsNaN(*v) || math.IsInf(*v, 0) {
		return "--"
	}
	return fmt.Sprintf("%.0f%s", *v, unit)
}

const (
	background = 0x15100c
	foreground = 0xf8f6ee
	muted      = 0xa69b8b
	cyan       = 0xdecf49
	orange     = 0x5dafff
	green      = 0x97d58b
	track      = 0x352a21
)

// Colors are Windows COLORREF (0x00BBGGRR).
func Render(s telemetry.Snapshot, role string) (*image.RGBA, error) {
	h := 180
	if role == "summary" {
		h = 480
	}
	if role != "summary" && role != "cpu" && role != "gpu" && role != "memory" {
		return nil, fmt.Errorf("unknown dashboard %q", role)
	}
	c, err := newCanvas(640, h)
	if err != nil {
		return nil, err
	}
	defer c.close()
	c.rect(0, 0, 640, h, background)
	usedRAM, totalRAM := memoryLabels(s.Memory)
	gpu := telemetry.GPU{}
	if len(s.GPUs) > 0 {
		gpu = s.GPUs[0]
	}
	usedVRAM, totalVRAM := vramLabels(gpu)
	bar := func(x, y, w int, v *float64, col uint32) {
		c.rect(x, y, w, 6, track)
		if v != nil && !math.IsNaN(*v) {
			n := int(math.Max(0, math.Min(100, *v)) * float64(w) / 100)
			c.rect(x, y, n, 6, col)
		}
	}
	text := func(x, y, size int, col uint32, str string) { c.text(x, y, size, col, str) }
	if role == "summary" {
		text(28, 18, 22, muted, "SYSTEM / LIVE")
		text(460, 18, 22, muted, s.Time.Format("15:04:05"))
		rows := []struct {
			label  string
			value  *float64
			detail string
			col    uint32
		}{
			{"CPU", s.CPU.UsagePercent, "TEMP " + metric(s.CPU.TemperatureC, " C"), cyan},
			{"GPU", gpu.UsagePercent, "TEMP " + metric(gpu.TemperatureC, " C") + "   POWER " + metric(gpu.PowerW, " W"), orange},
			{"RAM", s.Memory.UsagePercent, usedRAM + " / " + totalRAM + " GiB", green},
		}
		for i, r := range rows {
			y := 70 + i*122
			text(28, y, 24, r.col, r.label)
			text(27, y+28, 22, muted, r.detail)
			text(465, y-8, 56, foreground, metric(r.value, "%"))
			bar(28, y+85, 584, r.value, r.col)
		}
		text(28, 449, 17, muted, "GPU VRAM  "+usedVRAM+" / "+totalVRAM+" GiB")
	} else {
		var title, detail, extra string
		var v *float64
		var accent uint32
		switch role {
		case "cpu":
			title = "CPU / PROCESSOR"
			v = s.CPU.UsagePercent
			accent = cyan
			detail = "TOTAL PROCESSOR USAGE"
			extra = "TEMP  " + metric(s.CPU.TemperatureC, " C")
		case "gpu":
			title = "GPU / GRAPHICS"
			v = gpu.UsagePercent
			accent = orange
			detail = "TEMP " + metric(gpu.TemperatureC, " C") + "    " + metric(gpu.PowerW, " W")
			extra = "VRAM " + usedVRAM + " / " + totalVRAM + " GiB"
		case "memory":
			title = "RAM / SYSTEM"
			v = s.Memory.UsagePercent
			accent = green
			detail = usedRAM + " GiB USED"
			extra = totalRAM + " GiB TOTAL"
		}
		c.rect(0, 0, 6, 180, accent)
		text(26, 15, 22, accent, title)
		text(25, 49, 24, muted, detail)
		text(26, 103, 22, foreground, extra)
		text(430, 32, 78, foreground, metric(v, "%"))
		bar(26, 151, 588, v, accent)
	}
	return c.image()
}

func memoryLabels(m telemetry.Memory) (string, string) {
	if m.UsagePercent == nil {
		return "--", "--"
	}
	return fmt.Sprintf("%.1f", float64(m.UsedBytes)/(1<<30)), fmt.Sprintf("%.1f", float64(m.TotalBytes)/(1<<30))
}

func vramLabels(g telemetry.GPU) (string, string) {
	used, total := "--", "--"
	if g.MemoryUsedBytes != nil {
		used = fmt.Sprintf("%.1f", float64(*g.MemoryUsedBytes)/(1<<30))
	}
	if g.MemoryTotalBytes != nil {
		total = fmt.Sprintf("%.1f", float64(*g.MemoryTotalBytes)/(1<<30))
	}
	return used, total
}
