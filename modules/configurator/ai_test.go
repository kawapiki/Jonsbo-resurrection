package configurator

import (
	"context"
	"encoding/json"
	ai "github.com/kawapiki/Jonsbo-resurrection/modules/aisubscriptions"
	"image/png"
	"testing"
	"time"

	data "github.com/kawapiki/Jonsbo-resurrection/pkg/aisubscriptions"
	"github.com/kawapiki/Jonsbo-resurrection/pkg/module"
)

func TestAIOverlayValidationAndLegacyDefaults(t *testing.T) {
	l := Themes(640, 180)[0].Layout
	var o Overlay
	if err := json.Unmarshal([]byte(`{"id":"ai","type":"ai-provider","x":10,"y":10,"w":300,"h":150,"color":"#FFFFFF","opacity":1,"font_size":14,"min":0,"max":100}`), &o); err != nil {
		t.Fatal(err)
	}
	l.Overlays = []Overlay{o}
	if err := ValidateLayout(l); err != nil {
		t.Fatalf("default provider preferences: %v", err)
	}
	for _, fields := range []string{`"provider":"claude","detail":"compact","animate":false`, `"provider":"openai","detail":"expanded"`} {
		json.Unmarshal([]byte(`{`+fields+`}`), &l.Overlays[0])
		if err := ValidateLayout(l); err != nil {
			t.Fatal(err)
		}
	}
	for _, fields := range []string{`"provider":"unknown"`, `"provider":"openai","detail":"unknown"`} {
		json.Unmarshal([]byte(`{`+fields+`}`), &l.Overlays[0])
		if ValidateLayout(l) == nil {
			t.Fatalf("accepted %s", fields)
		}
	}
	for _, metric := range []string{"ai.openai.weekly", "ai.openai.tokens", "ai.openai.sessions", "ai.claude.weekly", "ai.claude.tokens", "ai.claude.sessions"} {
		l.Overlays[0] = Overlay{ID: "value", Type: "metric", Metric: metric, W: 100, H: 50, Color: "#FFFFFF", Opacity: 1, FontSize: 14, Max: 100}
		if err := ValidateLayout(l); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAIWidgetServerPreviewAndCanvasAgree(t *testing.T) {
	now := time.Now().UTC()
	tokens := int64(12345)
	off := false
	state := data.State{Providers: []data.Provider{{ID: "claude", AccountKey: "linked", Connection: "connected", UpdatedAt: now, WeeklyTokens: &tokens, Coverage: "Observed local", UsageLabel: "Tokens this week"}}}
	encoded, _ := json.Marshal(state)
	m, err := New(t.TempDir(), &fakeSource{states: []module.State{{Module: module.Descriptor{ID: "ai-subscriptions"}, Status: "running", UpdatedAt: now, Data: encoded}}})
	if err != nil {
		t.Fatal(err)
	}
	o := Overlay{ID: "ai", Type: "ai-provider", Provider: "claude", Detail: "expanded", Animate: &off, X: 20, Y: 15, W: 400, H: 300, Color: "#FFFFFF", Opacity: 1, FontSize: 14, Max: 100}
	l := Themes(640, 480)[0].Layout
	l.Overlays = []Overlay{o}
	native, err := m.render(context.Background(), l, now)
	if err != nil {
		t.Fatal(err)
	}
	o.X = 0
	o.Y = 0
	w := request(t, m, &fakeDisplays{}, "POST", "/ai-widget.png", o)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	browser, err := png.Decode(w.Body)
	if err != nil {
		t.Fatal(err)
	}
	want := ai.DrawWidgetDetail(state, "claude", "expanded", 400, 300, now, false)
	for y := 0; y < 300; y++ {
		for x := 0; x < 400; x++ {
			wr, wg, wb, wa := want.At(x, y).RGBA()
			br, bg, bb, ba := browser.At(x, y).RGBA()
			nr, ng, nb, na := native.At(x+20, y+15).RGBA()
			if wr != br || wg != bg || wb != bb || wa != ba || wr != nr || wg != ng || wb != nb || wa != na {
				t.Fatalf("widget rendering drift at %d,%d", x, y)
			}
		}
	}
	bad := o
	bad.Provider = "other"
	if w := request(t, m, &fakeDisplays{}, "POST", "/ai-widget.png", bad); w.Code != 400 {
		t.Fatalf("bad provider: %d", w.Code)
	}
	bad = o
	bad.Type = "metric"
	if w := request(t, m, &fakeDisplays{}, "POST", "/ai-widget.png", bad); w.Code != 400 {
		t.Fatalf("bad type: %d", w.Code)
	}
}

func TestAIWidgetPreferencesSurviveSavedLayoutRestart(t *testing.T) {
	m := fresh(t)
	off := false
	l := Themes(640, 180)[0].Layout
	l.ID = "ai-fan"
	l.Overlays = []Overlay{{ID: "ai", Type: "ai-provider", Provider: "claude", Animate: &off, Detail: "compact", W: 360, H: 150, Color: "#FFFFFF", FontSize: 14, Max: 100, Opacity: 1}}
	if w := request(t, m, &fakeDisplays{}, "PUT", "/layouts/ai-fan", l); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	reopened, err := New(m.dir, &fakeSource{})
	if err != nil {
		t.Fatal(err)
	}
	stored := reopened.layouts["ai-fan"].Overlays[0]
	if stored.Provider != "claude" || stored.Detail != "compact" || stored.Animate == nil || *stored.Animate {
		t.Fatalf("preferences lost: %+v", stored)
	}
	if err := ValidateLayout(reopened.layouts["default-pump"]); err != nil {
		t.Fatalf("legacy layout no longer valid: %v", err)
	}
}

func TestAIMetricsUseProviderFreshnessAndAccountScope(t *testing.T) {
	now := time.Now().UTC()
	percent := 31.0
	minutes := int64(10080)
	tokens := int64(450)
	state := data.State{Providers: []data.Provider{{ID: "openai", AccountKey: "linked", Connection: "connected", UpdatedAt: now.Add(-60 * time.Second), WeeklyTokens: &tokens, Capabilities: data.Capabilities{LocalSessions: true}, Quotas: []data.Quota{{UsedPercent: &percent, WindowMinutes: &minutes, ObservedAt: now.Add(-60 * time.Second)}}}}, Sessions: []data.Session{{Provider: "openai", AccountKey: "linked", Activity: "running"}, {Provider: "openai", Activity: "running"}, {Provider: "openai", AccountKey: "other", Activity: "running"}}}
	encoded, _ := json.Marshal(state)
	source := &fakeSource{states: []module.State{{Module: module.Descriptor{ID: "ai-subscriptions"}, Status: "running", UpdatedAt: now, Data: encoded}}}
	m, err := New(t.TempDir(), source)
	if err != nil {
		t.Fatal(err)
	}
	m.sample(now)
	values := m.history[len(m.history)-1].Values
	for id, want := range map[string]float64{"ai.openai.weekly": 31, "ai.openai.tokens": 450, "ai.openai.sessions": 1} {
		if values[id] == nil || *values[id] != want {
			t.Fatalf("%s=%v want %v", id, values[id], want)
		}
	}
	if values["ai.claude.tokens"] != nil || values["cpu.usage"] != nil {
		t.Fatal("absent data became zero")
	}
	state.Providers[0].Stale = true
	encoded, _ = json.Marshal(state)
	source.states[0].Data = encoded
	m.sample(now)
	if m.metrics[9].Value != nil {
		t.Fatal("stale account shown live")
	}
}
