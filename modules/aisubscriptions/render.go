package aisubscriptions

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"
	"time"

	data "github.com/kawapiki/Jonsbo-resurrection/pkg/aisubscriptions"
)

var background = color.RGBA{14, 20, 27, 255}
var panel = color.RGBA{22, 31, 40, 255}
var foreground = color.RGBA{231, 239, 240, 255}
var muted = color.RGBA{143, 160, 173, 255}
var track = color.RGBA{44, 58, 68, 255}

// DrawWidget returns an owned frame suitable for both physical displays and PNG previews.
func DrawWidget(s data.State, provider string, width, height int, now time.Time, animate bool) image.Image {
	detail := "compact"
	if height > 180 {
		detail = "expanded"
	}
	return DrawWidgetDetail(s, provider, detail, width, height, now, animate)
}

// DrawWidgetDetail accepts compact/expanded; an empty provider selects OpenAI.
// Dimensions outside the module image contract produce an empty image.
func DrawWidgetDetail(s data.State, provider, detail string, width, height int, now time.Time, animate bool) image.Image {
	if width < 1 || height < 1 || width > 2048 || height > 2048 || width*height > 2000000 {
		return image.NewRGBA(image.Rectangle{})
	}
	if provider == "" {
		provider = data.OpenAI
	}
	logicalW, logicalH := width, height
	logicalW = max(360, width)
	logicalH = max(164, height)
	canvas := image.NewRGBA(image.Rect(0, 0, logicalW, logicalH))
	fill(canvas, canvas.Bounds(), background)
	switch provider {
	case "overview":
		text(canvas, 18, 14, "AI ACTIVITY", 21, foreground)
		providerCard(canvas, s, data.OpenAI, image.Rect(12, 52, logicalW-12, 252), now, animate)
		providerCard(canvas, s, data.Claude, image.Rect(12, 268, logicalW-12, 468), now, animate)
	case "sessions":
		text(canvas, 18, 16, "OBSERVED SESSIONS", 21, foreground)
		sessionRows(canvas, s, "", 58, 102, max(1, (logicalH-58)/102))
	default:
		cardHeight := logicalH - 8
		headingY, sessionY, sessionStep := 236, 270, 100
		if detail == "expanded" && logicalH >= 300 {
			cardHeight = 216
			if logicalH < 360 {
				cardHeight = 164
				headingY = 184
				sessionY = 212
				sessionStep = 88
			}
		}
		providerCard(canvas, s, provider, image.Rect(0, 0, logicalW, cardHeight+8), now, animate)
		if detail == "expanded" && logicalH >= 300 {
			text(canvas, 16, headingY, "OBSERVED SESSIONS", 21, muted)
			sessionRows(canvas, s, provider, sessionY, sessionStep, max(1, (logicalH-sessionY)/sessionStep))
		}
	}
	if logicalW == width && logicalH == height {
		return canvas
	}
	out := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			out.SetRGBA(x, y, canvas.RGBAAt(x*logicalW/width, y*logicalH/height))
		}
	}
	return out
}
func providerState(s data.State, id string) data.Provider {
	for _, p := range s.Providers {
		if p.ID == id {
			return p
		}
	}
	return data.Provider{ID: id, Connection: "disconnected", UsageLabel: "Tokens this week", Coverage: "Unavailable"}
}
func weeklyQuota(q []data.Quota) *data.Quota {
	for i := range q {
		if q[i].WindowMinutes != nil && *q[i].WindowMinutes == 7*24*60 && !strings.HasPrefix(q[i].ID, "seven_day_") {
			return &q[i]
		}
	}
	return nil
}
func number(n *int64) string {
	if n == nil {
		return "--"
	}
	return fmt.Sprintf("%d", *n)
}
func clip(s string, n int) string {
	r := []rune(s)
	if n < 1 {
		return ""
	}
	if len(r) > n {
		if n > 3 {
			return string(r[:n-3]) + "..."
		}
		return string(r[:n])
	}
	return s
}
func sessionRows(im *image.RGBA, s data.State, id string, y, step, limit int) {
	count := 0
	for _, session := range s.Sessions {
		if id != "" && session.Provider != id {
			continue
		}
		if count >= limit {
			break
		}
		top := y + count*step
		title := session.Title
		if title == "" {
			title = session.Model
		}
		if title == "" {
			title = session.ID
		}
		group := session.Provider
		if session.AccountKey == "" {
			group += " / observed local"
		}
		activity := session.Activity
		if session.Stale {
			activity = "unknown / stale"
		}
		text(im, 18, top, clip(group+"  "+activity, (im.Bounds().Dx()-36)/12), 14, muted)
		text(im, 18, top+23, clip(title, (im.Bounds().Dx()-36)/18), 21, foreground)
		usage := "TOKENS " + number(session.ConsumedTokens) + "  CONTEXT --"
		var percent *float64
		if session.Context != nil {
			c := session.Context
			usage = "TOKENS " + number(session.ConsumedTokens) + "  CONTEXT " + number(c.Used) + " / " + number(c.Limit)
			percent = c.UsedPercent
			if percent == nil && c.Used != nil && c.Limit != nil && *c.Limit > 0 {
				v := float64(*c.Used) * 100 / float64(*c.Limit)
				percent = &v
			}
		}
		if step >= 80 {
			text(im, 18, top+55, clip(usage, (im.Bounds().Dx()-36)/12), 14, muted)
			rect := image.Rect(18, top+80, im.Bounds().Dx()-18, top+86)
			fill(im, rect, track)
			if percent != nil {
				bar(im, rect, *percent, color.RGBA{102, 170, 204, 255})
			}
		} else {
			text(im, 18, top+49, clip(usage, (im.Bounds().Dx()-36)/12), 14, muted)
		}
		count++
	}
	if count == 0 {
		text(im, 18, y, "NO OBSERVED SESSIONS", 14, muted)
	}
}
func fill(im *image.RGBA, r image.Rectangle, c color.RGBA) {
	draw.Draw(im, r, &image.Uniform{C: c}, image.Point{}, draw.Src)
}
func paintLogoFit(dst *image.RGBA, logo image.Image, rect image.Rectangle) {
	bounds := logo.Bounds()
	if bounds.Empty() || rect.Empty() {
		return
	}
	scale := math.Min(float64(rect.Dx())/float64(bounds.Dx()), float64(rect.Dy())/float64(bounds.Dy()))
	w, h := max(1, int(float64(bounds.Dx())*scale)), max(1, int(float64(bounds.Dy())*scale))
	x, y := rect.Min.X+(rect.Dx()-w)/2, rect.Min.Y+(rect.Dy()-h)/2
	mark := image.NewRGBA(image.Rect(0, 0, w, h))
	for row := 0; row < h; row++ {
		for col := 0; col < w; col++ {
			mark.Set(col, row, logo.At(bounds.Min.X+col*bounds.Dx()/w, bounds.Min.Y+row*bounds.Dy()/h))
		}
	}
	draw.Draw(dst, image.Rect(x, y, x+w, y+h), mark, image.Point{}, draw.Over)
}
func bar(im *image.RGBA, r image.Rectangle, p float64, c color.RGBA) {
	p = math.Max(0, math.Min(100, p))
	r.Max.X = r.Min.X + int(float64(r.Dx())*p/100)
	fill(im, r, c)
}

// The project's portable 5x7 LCD font; unsupported Unicode renders as '?'.
func text(im *image.RGBA, x, y int, s string, size int, c color.RGBA) {
	scale := max(1, size/7)
	for _, r := range strings.ToUpper(s) {
		glyph, ok := glyphs[r]
		if !ok {
			glyph = glyphs['?']
		}
		if x+5*scale > im.Bounds().Dx() {
			break
		}
		for row, bits := range glyph {
			for col := 0; col < 5; col++ {
				if bits&(1<<uint(4-col)) != 0 {
					fill(im, image.Rect(x+col*scale, y+row*scale, x+(col+1)*scale, y+(row+1)*scale), c)
				}
			}
		}
		x += 6 * scale
	}
}

var glyphs = map[rune][7]uint8{
	' ': {0, 0, 0, 0, 0, 0, 0}, 'A': {14, 17, 17, 31, 17, 17, 17}, 'B': {30, 17, 17, 30, 17, 17, 30}, 'C': {14, 17, 16, 16, 16, 17, 14}, 'D': {30, 17, 17, 17, 17, 17, 30}, 'E': {31, 16, 16, 30, 16, 16, 31}, 'F': {31, 16, 16, 30, 16, 16, 16}, 'G': {14, 17, 16, 23, 17, 17, 15}, 'H': {17, 17, 17, 31, 17, 17, 17}, 'I': {14, 4, 4, 4, 4, 4, 14}, 'J': {7, 2, 2, 2, 18, 18, 12}, 'K': {17, 18, 20, 24, 20, 18, 17}, 'L': {16, 16, 16, 16, 16, 16, 31}, 'M': {17, 27, 21, 21, 17, 17, 17}, 'N': {17, 25, 21, 19, 17, 17, 17}, 'O': {14, 17, 17, 17, 17, 17, 14}, 'P': {30, 17, 17, 30, 16, 16, 16}, 'Q': {14, 17, 17, 17, 21, 18, 13}, 'R': {30, 17, 17, 30, 20, 18, 17}, 'S': {15, 16, 16, 14, 1, 1, 30}, 'T': {31, 4, 4, 4, 4, 4, 4}, 'U': {17, 17, 17, 17, 17, 17, 14}, 'V': {17, 17, 17, 17, 17, 10, 4}, 'W': {17, 17, 17, 21, 21, 21, 10}, 'X': {17, 17, 10, 4, 10, 17, 17}, 'Y': {17, 17, 10, 4, 4, 4, 4}, 'Z': {31, 1, 2, 4, 8, 16, 31},
	'0': {14, 17, 19, 21, 25, 17, 14}, '1': {4, 12, 4, 4, 4, 4, 14}, '2': {14, 17, 1, 2, 4, 8, 31}, '3': {30, 1, 1, 14, 1, 1, 30}, '4': {2, 6, 10, 18, 31, 2, 2}, '5': {31, 16, 16, 30, 1, 1, 30}, '6': {14, 16, 16, 30, 17, 17, 14}, '7': {31, 1, 2, 4, 8, 8, 8}, '8': {14, 17, 17, 14, 17, 17, 14}, '9': {14, 17, 17, 15, 1, 1, 14},
	'.': {0, 0, 0, 0, 0, 6, 6}, ':': {0, 6, 6, 0, 6, 6, 0}, '-': {0, 0, 0, 31, 0, 0, 0}, '_': {0, 0, 0, 0, 0, 0, 31}, '/': {1, 1, 2, 4, 8, 16, 16}, '%': {25, 25, 2, 4, 8, 19, 19}, '?': {14, 17, 1, 2, 4, 0, 4}, '+': {0, 4, 4, 31, 4, 4, 0}, ';': {0, 6, 6, 0, 6, 6, 4}, '|': {4, 4, 4, 4, 4, 4, 4},
}
