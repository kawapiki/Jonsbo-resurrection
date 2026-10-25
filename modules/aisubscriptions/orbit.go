package aisubscriptions

import (
	"fmt"
	"github.com/kawapiki/Jonsbo-resurrection/internal/imageutil"
	data "github.com/kawapiki/Jonsbo-resurrection/pkg/aisubscriptions"
	"image"
	"image/color"
	"math"
)

// Compact wording for the same statuses used by the rectangular provider cards.
func compactProviderStatus(status string) string {
	switch status {
	case "CONNECTED / IDLE":
		return "Connected"
	case "WORKING":
		return "Working"
	case "NEEDS YOUR INPUT":
		return "Waiting"
	case "SIGN IN NEEDED":
		return "Sign in"
	case "NOT CONNECTED":
		return "Offline"
	default:
		return "Stale"
	}
}

func orbitText(im *image.RGBA, y int, value string, size, width int, c color.RGBA) {
	for size > 7 && len([]rune(value))*6*max(1, size/7) > width {
		size--
	}
	if len([]rune(value))*6 > width {
		value = clip(value, max(0, width/6))
	}
	w := len([]rune(value)) * 6 * max(1, size/7)
	text(im, max(0, (im.Bounds().Dx()-w)/2), y, value, size, c)
}
func orbitRing(im *image.RGBA, radius, thickness float64, value *float64, c color.RGBA) {
	ratio := 0.0
	if value != nil {
		ratio = *value / 100
	}
	track := color.RGBA{R: c.R / 6, G: c.G / 6, B: c.B / 6, A: c.A}
	imageutil.Ring(im, radius, thickness, ratio, c, track)
}

func orbitPercent(v *float64) string {
	if v == nil {
		return "--"
	}
	return fmt.Sprintf("%.0f%%", *v)
}

func drawOrbitWidget(s data.State, id, detail string, w, h int) image.Image {
	im := image.NewRGBA(image.Rect(0, 0, w, h))
	view := presentProvider(s, id)
	status := compactProviderStatus(view.status)
	var week, short *float64
	if view.quota != nil {
		week = view.quota.UsedPercent
	}
	for _, q := range view.quotas {
		if q.WindowMinutes != nil && *q.WindowMinutes == 300 && q.UsedPercent != nil {
			short = q.UsedPercent
			break
		}
	}
	name := "OpenAI"
	accent := color.RGBA{98, 220, 229, 255}
	if id == data.Claude {
		name = "Claude"
		accent = color.RGBA{255, 172, 98, 255}
	}
	statusColor := foreground
	if status == "Waiting" || status == "Sign in" {
		statusColor = color.RGBA{255, 199, 107, 255}
	} else if status == "Offline" || status == "Stale" {
		statusColor = muted
	}
	if detail == "strip" {
		// Native two-line status strip: no scaled-down desktop card.
		orbitText(im, 0, name, 7, w, accent)
		orbitText(im, min(13, max(0, h-7)), status, 14, w, statusColor)
		return im
	}
	radius := float64(min(w, h))/2 - 1
	orbitRing(im, radius, 5, week, accent)
	orbitRing(im, math.Max(0, radius-10), 3, short, color.RGBA{153, 169, 232, 255})
	// Text fits inside the inner ring at native 220px; smaller widgets use fewer pixels.
	scale := float64(min(w, h)) / 220
	yy := func(v int) int { return int(float64(v) * scale) }
	safe := max(1, int(float64(min(w, h))*.72))
	orbitText(im, yy(40), name, 14, safe, accent)
	orbitText(im, yy(70), status, 21, safe, statusColor)
	tokenText := compactNumber(view.tokens) + " tokens/week"
	if view.localTokens {
		tokenText = compactNumber(view.tokens) + " local tokens"
	}
	orbitText(im, yy(102), tokenText, 7, safe, muted)
	if view.scope == "PARTIAL / WEEK" {
		orbitText(im, yy(114), "Partial week", 7, safe, muted)
	}
	orbitText(im, yy(127), "5h "+orbitPercent(short), 14, safe, color.RGBA{153, 169, 232, 255})
	orbitText(im, yy(149), "7d "+orbitPercent(week), 14, safe, accent)
	footer := "Quota used"
	if view.localQuota {
		footer = "Local quota used"
	}
	orbitText(im, yy(176), footer, 7, safe, muted)
	return im
}
