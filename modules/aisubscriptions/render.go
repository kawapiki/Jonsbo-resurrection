package aisubscriptions

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"
	"time"

	"github.com/kawapiki/Jonsbo-resurrection/internal/aibrand"
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
	if width < 320 {
		logicalW = 320
		logicalH = max(180, height*320/width)
	}
	canvas := image.NewRGBA(image.Rect(0, 0, logicalW, logicalH))
	fill(canvas, canvas.Bounds(), background)
	switch provider {
	case "overview":
		text(canvas, 18, 12, "AI SUBSCRIPTIONS", 14, foreground)
		providerCard(canvas, s, data.OpenAI, image.Rect(12, 36, logicalW-12, 184), now, animate)
		providerCard(canvas, s, data.Claude, image.Rect(12, 194, logicalW-12, 342), now, animate)
		text(canvas, 18, 354, "OBSERVED SESSIONS", 14, muted)
		sessionRows(canvas, s, "", 382, 44, 2)
	case "sessions":
		text(canvas, 18, 16, "OBSERVED SESSIONS", 21, foreground)
		sessionRows(canvas, s, "", 54, 65, max(1, (logicalH-54)/65))
	default:
		providerCard(canvas, s, provider, image.Rect(8, 8, logicalW-8, min(logicalH-8, 172)), now, animate)
		if detail == "expanded" && logicalH > 212 {
			text(canvas, 16, 190, "OBSERVED SESSIONS", 14, muted)
			sessionRows(canvas, s, provider, 214, 58, max(1, (logicalH-214)/58))
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
func providerCard(im *image.RGBA, s data.State, id string, r image.Rectangle, now time.Time, animate bool) {
	p := providerState(s, id)
	fill(im, r, panel)
	accent := color.RGBA{90, 211, 179, 255}
	name := "OPENAI / CODEX"
	if id == data.Claude {
		accent = color.RGBA{217, 151, 117, 255}
		name = "CLAUDE"
	}
	live := p.Connection == "connected" && !p.Stale
	pulse := 1.0
	if live && animate {
		pulse = 0.70 + 0.30*(1+math.Sin(float64(now.UnixNano())/1e9*math.Pi))/2
	}
	markColor := muted
	if live {
		markColor = color.RGBA{uint8(float64(accent.R) * pulse), uint8(float64(accent.G) * pulse), uint8(float64(accent.B) * pulse), 255}
	}
	cx, cy := r.Min.X+22, r.Min.Y+26
	for y := -12; y <= 12; y++ {
		for x := -12; x <= 12; x++ {
			d := x*x + y*y
			if d >= 81 && d <= 144 {
				im.SetRGBA(cx+x, cy+y, markColor)
			}
		}
	}
	if logo := aibrand.Logo(id); logo != nil {
		paintLogoFit(im, logo, image.Rect(cx-8, cy-8, cx+8, cy+8))
	}
	text(im, r.Min.X+44, r.Min.Y+12, name, 14, foreground)
	status := p.Connection
	if p.Stale {
		status += " / STALE"
	}
	if len(p.Quotas) > 0 && !p.Quotas[0].ObservedAt.IsZero() {
		age := max(0, int(now.Sub(p.Quotas[0].ObservedAt).Seconds()))
		unit := "S"
		if age >= 86400 {
			age /= 86400
			unit = "D"
		} else if age >= 3600 {
			age /= 3600
			unit = "H"
		} else if age >= 60 {
			age /= 60
			unit = "M"
		}
		status += fmt.Sprintf(" / QUOTA %d%s AGO", age, unit)
	}
	text(im, r.Min.X+44, r.Min.Y+32, clip(status, (r.Dx()-52)/6), 7, muted)
	quotas := p.Quotas
	localQuota := false
	if weeklyQuota(quotas) == nil && len(p.LocalQuotas) > 0 {
		quotas = p.LocalQuotas
		localQuota = true
	}
	quota := weeklyQuota(quotas)
	quotaText := "--"
	if quota != nil && quota.UsedPercent != nil {
		quotaText = fmt.Sprintf("%.0f%%", *quota.UsedPercent)
	}
	text(im, r.Min.X+12, r.Min.Y+57, "7-DAY QUOTA  "+quotaText, 14, foreground)
	barRect := image.Rect(r.Min.X+12, r.Min.Y+78, r.Max.X-12, r.Min.Y+84)
	fill(im, barRect, track)
	if quota != nil && quota.UsedPercent != nil {
		bar(im, barRect, *quota.UsedPercent, accent)
	}
	reset := "RESET --"
	if quota != nil && quota.ResetsAt != nil {
		remaining := quota.ResetsAt.Sub(now)
		if remaining > 0 {
			hours := int(math.Ceil(remaining.Hours()))
			reset = fmt.Sprintf("RESET IN %dD %dH", hours/24, hours%24)
		} else {
			reset = "RESET DUE"
		}
	}
	for _, q := range quotas {
		if q.WindowMinutes != nil && *q.WindowMinutes == 300 && q.UsedPercent != nil {
			reset = fmt.Sprintf("5H %.0f%%  |  %s", *q.UsedPercent, reset)
			break
		}
	}
	if id == data.Claude && len(quotas) == 0 {
		reset = "LIMITS NOT REPORTED"
	}
	text(im, r.Min.X+12, r.Min.Y+90, reset, 7, muted)
	label := p.UsageLabel
	if label == "" {
		label = "Tokens this week"
	}
	tokens := p.WeeklyTokens
	localTokens := false
	if id == data.Claude && tokens == nil && p.LocalWeeklyTokens != nil {
		tokens = p.LocalWeeklyTokens
		label = "Local tokens this week"
		localTokens = true
	}
	text(im, r.Min.X+12, r.Min.Y+105, clip(label+": "+number(tokens), (r.Dx()-24)/6), 7, foreground)
	coverage := p.Coverage
	if coverage == "" {
		coverage = "Unavailable"
	}
	if localQuota || localTokens {
		coverage = "Observed Claude Code; partial"
	} else if id == data.Claude && tokens == nil && len(quotas) == 0 {
		coverage = "Claude Code usage feed required"
	}
	if id == data.Claude && len(p.Quotas) > 0 {
		coverage = "Subscription quota; tokens unavailable"
		if localTokens {
			coverage = "Subscription quota; local tokens partial"
		}
	}
	if p.Partial && !strings.Contains(strings.ToLower(coverage), "partial") {
		coverage += " / partial"
	}
	text(im, r.Min.X+12, r.Min.Y+121, clip(coverage, (r.Dx()-24)/6), 7, muted)
	// Activity is independent of the account connection indicator.
	activity := ""
	for _, session := range s.Sessions {
		if session.Provider == id && !session.Stale && member(session.Activity, "running", "waiting") {
			activity = session.Activity
			if activity == "running" {
				break
			}
		}
	}
	if activity != "" {
		a := accent
		if animate && activity == "running" {
			brightness := (1 + math.Sin(float64(now.UnixNano())/1e9*2*math.Pi)) / 2
			a = color.RGBA{uint8(float64(accent.R) * (.65 + .35*brightness)), uint8(float64(accent.G) * (.65 + .35*brightness)), uint8(float64(accent.B) * (.65 + .35*brightness)), 255}
		}
		x := r.Max.X - 24
		fill(im, image.Rect(x, r.Min.Y+16, x+8, r.Min.Y+24), a)
		if activity == "waiting" {
			fill(im, image.Rect(x+3, r.Min.Y+16, x+5, r.Min.Y+24), panel)
		}
	}
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
		text(im, 18, top, clip(group+"  "+activity, (im.Bounds().Dx()-36)/6), 7, muted)
		text(im, 18, top+11, clip(title, (im.Bounds().Dx()-36)/6), 7, foreground)
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
		if step >= 58 {
			text(im, 18, top+25, clip(usage, (im.Bounds().Dx()-36)/6), 7, muted)
			rect := image.Rect(18, top+40, im.Bounds().Dx()-18, top+44)
			fill(im, rect, track)
			if percent != nil {
				bar(im, rect, *percent, color.RGBA{102, 170, 204, 255})
			}
		} else {
			text(im, 18, top+23, clip(usage, (im.Bounds().Dx()-36)/6), 7, muted)
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
