package aisubscriptions

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"time"

	"github.com/kawapiki/Jonsbo-resurrection/internal/aibrand"
	data "github.com/kawapiki/Jonsbo-resurrection/pkg/aisubscriptions"
)

func compactNumber(n *int64) string {
	if n == nil {
		return "--"
	}
	for _, unit := range []struct {
		size   int64
		suffix string
	}{{1000000000, "B"}, {1000000, "M"}, {1000, "K"}} {
		if *n >= unit.size {
			return fmt.Sprintf("%.1f%s", float64(*n)/float64(unit.size), unit.suffix)
		}
	}
	return fmt.Sprintf("%d", *n)
}

func activityFor(s data.State, id string) string {
	activity := ""
	for _, session := range s.Sessions {
		if session.Provider == id && !session.Stale && member(session.Activity, "running", "waiting") {
			activity = session.Activity
			if activity == "running" {
				break
			}
		}
	}
	return activity
}

func providerCard(im *image.RGBA, s data.State, id string, r image.Rectangle, now time.Time, animate bool) {
	// Draw within the card so even a resized widget cannot bleed into its neighbours.
	r = r.Intersect(im.Bounds())
	if r.Empty() {
		return
	}
	card := im.SubImage(r).(*image.RGBA)
	p := providerState(s, id)
	accent := color.RGBA{115, 206, 191, 255}
	name := "CHATGPT / CODEX"
	if id == data.Claude {
		accent = color.RGBA{217, 119, 87, 255}
		name = "CLAUDE"
	}
	fill(card, r, panel)
	fill(card, image.Rect(r.Min.X, r.Min.Y, r.Min.X+4, r.Max.Y), accent)
	activity := activityFor(s, id)
	live := p.Connection == "connected" && !p.Stale
	status := "NOT CONNECTED"
	if live {
		status = "CONNECTED / IDLE"
	}
	if p.Stale {
		status = "LAST KNOWN / STALE"
	}
	if activity == "running" {
		status = "WORKING"
	} else if activity == "waiting" {
		status = "NEEDS YOUR INPUT"
	}
	if p.Connection == "reauthentication" {
		status = "SIGN IN NEEDED"
	}
	if p.Stale && activity != "" && p.Connection != "reauthentication" {
		status = "STALE / " + strings.ToUpper(activity)
	}
	quotas := p.Quotas
	localQuota := false
	if weeklyQuota(quotas) == nil && len(p.LocalQuotas) > 0 {
		quotas = p.LocalQuotas
		localQuota = true
	}
	quota := weeklyQuota(quotas)
	value := "--"
	if quota != nil && quota.UsedPercent != nil {
		value = fmt.Sprintf("%.0f%%", *quota.UsedPercent)
	}
	tokens := p.WeeklyTokens
	localTokens := false
	if id == data.Claude && tokens == nil && p.LocalWeeklyTokens != nil {
		tokens = p.LocalWeeklyTokens
		localTokens = true
	}
	quotaLabel := "7-DAY QUOTA"
	if localQuota {
		quotaLabel = "LOCAL QUOTA"
	}
	tokenLabel := "WEEKLY TOKENS"
	if localTokens {
		tokenLabel = "LOCAL TOKENS"
	} else if strings.Contains(strings.ToLower(p.UsageLabel), "observed") {
		tokenLabel = "OBSERVED TOKENS"
	}
	scope := "THIS WEEK"
	if localTokens || p.Partial {
		scope = "PARTIAL / WEEK"
	}
	if tokens == nil {
		scope = "NOT REPORTED"
	}
	reset := "RESET --"
	if quota != nil && quota.ResetsAt != nil {
		hours := int(math.Ceil(quota.ResetsAt.Sub(now).Hours()))
		if hours > 0 {
			reset = fmt.Sprintf("RESET %dD %dH", hours/24, hours%24)
		} else {
			reset = "RESET DUE"
		}
	}
	for _, q := range quotas {
		if q.WindowMinutes != nil && *q.WindowMinutes == 300 && q.UsedPercent != nil {
			reset = fmt.Sprintf("5H %.0f%% / %s", *q.UsedPercent, reset)
			break
		}
	}
	if quota != nil && !quota.ObservedAt.IsZero() {
		age := max(0, int(now.Sub(quota.ObservedAt).Minutes()))
		if age >= 5 {
			reset = fmt.Sprintf("READ %dM AGO / ", age) + reset
		}
	}
	x, y := r.Min.X, r.Min.Y
	wide := r.Dx() >= 520
	if wide {
		markSize := 104
		drawMascot(card, id, image.Rect(x+16, y+18, x+16+markSize, y+18+markSize), now, animate && (live || activity != ""), activity)
		left := x + 140
		right := x + max(350, r.Dx()*62/100)
		text(card, left, y+10, name, 21, foreground)
		text(card, left, y+38, status, 14, accent)
		text(card, left, y+64, quotaLabel, 14, muted)
		text(card, left, y+86, value, 42, foreground)
		text(card, right, y+64, clip(tokenLabel, (r.Max.X-right-12)/12), 14, muted)
		text(card, right, y+89, compactNumber(tokens), 28, foreground)
		text(card, right, y+122, scope, 14, muted)
		b := image.Rect(left, y+133, right-24, y+139)
		fill(card, b, track)
		if quota != nil && quota.UsedPercent != nil {
			bar(card, b, *quota.UsedPercent, accent)
		}
		text(card, x+16, y+146, clip(reset, (r.Dx()-32)/12), 14, muted)
		if r.Dy() >= 194 {
			coverage := p.Coverage
			if localQuota || localTokens {
				coverage = "OBSERVED CLAUDE CODE / PARTIAL"
			}
			if coverage == "" {
				coverage = "USAGE NOT REPORTED"
			}
			text(card, x+16, y+174, clip(coverage, (r.Dx()-32)/12), 14, muted)
		}
	} else {
		drawMascot(card, id, image.Rect(x+10, y+4, x+70, y+64), now, animate && (live || activity != ""), activity)
		text(card, x+80, y+10, name, 14, foreground)
		text(card, x+80, y+34, clip(status, (r.Dx()-92)/12), 14, accent)
		left, right := x+14, x+r.Dx()/2+6
		text(card, left, y+66, quotaLabel, 14, muted)
		text(card, left, y+88, value, 28, foreground)
		text(card, right, y+66, clip(tokenLabel, (r.Max.X-right-8)/12), 14, muted)
		text(card, right, y+90, compactNumber(tokens), 21, foreground)
		b := image.Rect(left, y+122, r.Max.X-14, y+128)
		fill(card, b, track)
		if quota != nil && quota.UsedPercent != nil {
			bar(card, b, *quota.UsedPercent, accent)
		}
		text(card, left, y+140, clip(reset, (r.Dx()-28)/12), 14, muted)
		if r.Dy() >= 182 {
			text(card, left, y+166, scope, 14, muted)
		}
	}
}

// Every pose is drawn for the current frame. No video, sprite sheet or browser is needed.
func drawMascot(dst *image.RGBA, id string, r image.Rectangle, now time.Time, animate bool, activity string) {
	phase := 0.0
	if animate {
		phase = float64(now.UnixMilli()%12000) / 1000 * 2.4
	}
	if id == data.Claude {
		unit := float64(r.Dx()) / 20
		bob := 0
		stride := 0
		if animate {
			bob = int(math.Round(math.Sin(phase) * unit * .45))
			stride = int(math.Round(math.Sin(phase) * unit * .6))
		}
		if activity == "waiting" {
			stride = 0
			bob = int(math.Round(math.Sin(phase*.5) * unit * .3))
		}
		if !animate {
			bob = 0
		}
		clay := color.RGBA{217, 119, 87, 255}
		block := func(x, y, w, h float64, ox, oy int, c color.RGBA) {
			fill(dst, image.Rect(r.Min.X+int(x*unit)+ox, r.Min.Y+int(y*unit)+oy, r.Min.X+int((x+w)*unit)+ox, r.Min.Y+int((y+h)*unit)+oy), c)
		}
		block(4, 3, 12, 8, 0, bob, clay)
		arm := 0
		if activity == "waiting" && animate {
			arm = -int((1 + math.Sin(phase*.5)) * unit)
		}
		block(1, 6, 3, 3, 0, bob+arm, clay)
		block(16, 6, 3, 3, 0, bob-arm, clay)
		for i, x := range []float64{4, 7, 11, 14} {
			offset := stride
			if i%2 == 1 {
				offset = -stride
			}
			block(x, 11, 2, 3, offset, bob, clay)
		}
		eye := color.RGBA{36, 27, 24, 255}
		eyeHeight := 1.5
		if animate && int(phase*2)%23 == 0 {
			eyeHeight = .5
		}
		block(5.5, 4.5, 1.5, eyeHeight, 0, bob, eye)
		block(13, 4.5, 1.5, eyeHeight, 0, bob, eye)
		return
	}
	logo := aibrand.Logo(data.OpenAI)
	if logo == nil {
		return
	}
	angle := 0.0
	scale := .82
	if animate {
		switch activity {
		case "running":
			angle = phase * .12
			scale = .82 + .04*math.Sin(phase)
		case "waiting":
			angle = .10 * math.Sin(phase*.5)
		default:
			scale = .82 + .025*math.Sin(phase*.5)
		}
	}
	// Supersampling keeps the blossom edges smooth during rotation.
	b := logo.Bounds()
	cx, cy := float64(r.Min.X+r.Max.X)/2, float64(r.Min.Y+r.Max.Y)/2
	cos, sin := math.Cos(angle), math.Sin(angle)
	size := float64(r.Dx()) * scale
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			var coverage uint32
			for _, dy := range []float64{.25, .75} {
				for _, dx := range []float64{.25, .75} {
					px, py := float64(x)+dx-cx, float64(y)+dy-cy
					u, v := (px*cos+py*sin)/size+.5, (-px*sin+py*cos)/size+.5
					if u >= 0 && u < 1 && v >= 0 && v < 1 {
						cr, cg, cb, ca := logo.At(b.Min.X+int(u*float64(b.Dx())), b.Min.Y+int(v*float64(b.Dy()))).RGBA()
						// Render the official monochrome silhouette in white on a dark screen.
						// The white icon background contributes no coverage.
						coverage += ca - (cr+cg+cb)/3
					}
				}
			}
			if coverage > 0 {
				old := dst.RGBAAt(x, y)
				a := float64(coverage) / (4 * 65535)
				dst.SetRGBA(x, y, color.RGBA{uint8(255*a + float64(old.R)*(1-a)), uint8(255*a + float64(old.G)*(1-a)), uint8(255*a + float64(old.B)*(1-a)), 255})
			}
		}
	}
}
