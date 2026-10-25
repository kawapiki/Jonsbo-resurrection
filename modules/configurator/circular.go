package configurator

import (
	"github.com/kawapiki/Jonsbo-resurrection/internal/imageutil"
	"image"
	"image/color"
	"math"
)

func canvasDimensions(w, h int) bool {
	return (w == 640 && (h == 480 || h == 180)) || (w == 240 && h == 240)
}

// Native compositions reserve the rim for rings and the center for readable data.
func circularThemes() []Theme {
	const ink = "#080D14"
	const cpu = "#62DCE5"
	const gpu = "#FFAC62"
	const ram = "#C4A0FF"
	makePart := func(id, typ, metric, label string, x, y, w, h, size int, c string) Overlay {
		return Overlay{ID: id, Type: typ, Metric: metric, Label: label, X: x, Y: y, W: w, H: h, FontSize: size, Color: c, Min: 0, Max: 100, Opacity: 1}
	}
	ring := func(id, metric string, inset int, c string) Overlay {
		return makePart(id, "ring", metric, "", inset, inset, 240-inset*2, 240-inset*2, 7, c)
	}
	num := func(id, metric, label string, x, y, w, h, size int, c string) Overlay {
		return makePart(id, "number", metric, label, x, y, w, h, size, c)
	}
	trace := func(id, metric string, x, y, w, h int, c string) Overlay {
		return makePart(id, "sparkline", metric, "", x, y, w, h, 7, c)
	}
	label := func(id, value string, x, y, w int, c string) Overlay {
		return makePart(id, "text", "", value, x, y, w, 9, 7, c)
	}
	provider := func(id, p, detail string, x, y, w, h int) Overlay {
		o := makePart(id, "ai-provider", "", "", x, y, w, h, 14, cpu)
		o.Provider = p
		o.Detail = detail
		return o
	}
	theme := func(id, name, desc string, parts ...Overlay) Theme {
		return Theme{ID: id, Name: name, Description: desc, Layout: Layout{ID: id, Name: name, Width: 240, Height: 240, Background: Background{Color: ink, Opacity: 1}, Overlays: parts}}
	}
	return []Theme{
		theme("cyclops-thermal", "Thermal dial", "CPU temperature, load and a two-minute trace.",
			ring("temperature", "cpu.temp", 6, cpu), num("cpu", "cpu.temp", "CPU", 45, 47, 150, 70, 42, cpu),
			trace("trace", "cpu.temp", 45, 127, 150, 31, cpu), label("history", "TEMP / 2 MIN", 84, 163, 90, cpu),
			num("load", "cpu.usage", "LOAD", 78, 180, 84, 35, 14, cpu)),
		theme("cyclops-graphics", "Graphics dial", "GPU heat, utilization and recent temperature.",
			ring("temperature", "gpu.temp", 6, gpu), num("gpu", "gpu.temp", "GPU", 45, 47, 150, 70, 42, gpu),
			trace("trace", "gpu.temp", 45, 127, 150, 31, gpu), label("history", "TEMP / 2 MIN", 84, 163, 90, gpu),
			num("load", "gpu.usage", "LOAD", 78, 180, 84, 35, 14, gpu)),
		theme("cyclops-duo", "Twin temperatures", "Two temperature rings and paired CPU/GPU readings.",
			ring("cpu-ring", "cpu.temp", 6, cpu), ring("gpu-ring", "gpu.temp", 17, gpu),
			num("cpu", "cpu.temp", "CPU", 45, 56, 72, 58, 21, cpu), num("gpu", "gpu.temp", "GPU", 123, 56, 72, 58, 21, gpu),
			trace("cpu-trace", "cpu.temp", 50, 124, 140, 28, cpu), trace("gpu-trace", "gpu.temp", 50, 157, 140, 28, gpu), label("history", "TEMP / 2 MIN", 84, 191, 85, "#9AAABC")),
		theme("cyclops-memory", "Memory orbit", "RAM use, free memory and a two-minute trace.",
			ring("ram-ring", "ram.usage", 6, ram), num("ram", "ram.usage", "MEMORY", 45, 47, 150, 70, 42, ram),
			trace("trace", "ram.usage", 45, 127, 150, 31, ram), num("free", "ram.free", "FREE", 65, 171, 110, 43, 21, ram)),
		theme("cyclops-load", "Processor pulse", "CPU load in large type with a wide activity trace.",
			ring("load-ring", "cpu.usage", 6, cpu), num("load", "cpu.usage", "CPU LOAD", 45, 47, 150, 70, 42, cpu),
			trace("trace", "cpu.usage", 43, 126, 154, 40, cpu), num("heat", "cpu.temp", "TEMP", 78, 180, 84, 35, 14, cpu)),
		theme("cyclops-system", "System orbit", "CPU/GPU temperature rings, memory and CPU activity.",
			ring("cpu-ring", "cpu.temp", 6, cpu), ring("gpu-ring", "gpu.temp", 17, gpu),
			num("cpu", "cpu.temp", "CPU", 45, 48, 72, 58, 21, cpu), num("gpu", "gpu.temp", "GPU", 123, 48, 72, 58, 21, gpu),
			trace("trace", "cpu.usage", 47, 116, 146, 32, cpu), label("history", "CPU / 2 MIN", 87, 154, 80, cpu),
			num("ram", "ram.usage", "RAM", 78, 176, 84, 38, 21, ram)),
		theme("cyclops-openai", "OpenAI orbit", "Observed activity with 5-hour and 7-day quota rings.", provider("openai", "openai", "orbit", 10, 10, 220, 220)),
		theme("cyclops-claude", "Claude orbit", "Claude activity and available quota windows.", provider("claude", "claude", "orbit", 10, 10, 220, 220)),
		theme("cyclops-ai", "AI duet", "Both providers with weekly usage rings and activity status.",
			ring("openai-ring", "ai.openai.weekly", 6, cpu), ring("claude-ring", "ai.claude.weekly", 17, gpu),
			provider("openai", "openai", "strip", 45, 49, 150, 37), num("openai-use", "ai.openai.weekly", "7D USED", 78, 90, 84, 34, 14, cpu),
			provider("claude", "claude", "strip", 45, 133, 150, 37), num("claude-use", "ai.claude.weekly", "7D USED", 78, 174, 84, 34, 14, gpu)),
		theme("cyclops-console", "System + AI", "CPU/GPU rings, temperature, history and both AI statuses.",
			ring("cpu-ring", "cpu.temp", 6, cpu), ring("gpu-ring", "gpu.temp", 17, gpu),
			num("cpu", "cpu.temp", "CPU", 45, 43, 72, 54, 21, cpu), num("gpu", "gpu.temp", "GPU", 123, 43, 72, 54, 21, gpu),
			trace("trace", "cpu.usage", 48, 103, 144, 24, cpu), label("history", "CPU / 2 MIN", 87, 132, 80, cpu),
			provider("openai", "openai", "strip", 48, 144, 144, 27), provider("claude", "claude", "strip", 65, 177, 110, 27)),
	}
}

func centeredText(im *image.RGBA, y int, value string, size, width int, fg color.RGBA) {
	for size > 7 && textWidth(value, size) > width {
		size--
	}
	text(im, max(0, (im.Bounds().Dx()-textWidth(value, size))/2), y, value, size, fg)
}

func circularWidget(layer *image.RGBA, o Overlay, label, value string, ratio float64, available bool, fg color.RGBA) {
	cy, cx := float64(o.H)/2, float64(o.W)/2
	radius := math.Min(cx, cy) - 3
	if o.Type == "radial" {
		track := color.RGBA{R: fg.R / 5, G: fg.G / 5, B: fg.B / 5, A: fg.A}
		if !available {
			ratio = 0
		}
		imageutil.Ring(layer, radius, max(5, radius*.07), ratio, fg, track)
	}

	safeWidth := o.W - 8
	if o.Type == "radial" {
		safeWidth = int(radius * 1.5)
	}
	size := o.FontSize
	for size > 7 && textWidth(value, size) > safeWidth {
		size--
	}
	valueHeight := max(1, size/7) * 7
	y := max(18, (o.H-valueHeight)/2+7)
	centeredText(layer, y-23, label, min(14, o.FontSize), safeWidth, fg)
	centeredText(layer, y, value, size, safeWidth, fg)
}
