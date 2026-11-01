package aisubscriptions

import (
	"bytes"
	data "github.com/kawapiki/Jonsbo-resurrection/pkg/aisubscriptions"
	"image"
	"image/png"
	"testing"
	"time"
)

func orbitBytes(im image.Image) []byte { var b bytes.Buffer; png.Encode(&b, im); return b.Bytes() }
func TestOrbitIsNativeAndTransparentOutsideCircle(t *testing.T) {
	im := DrawWidgetDetail(data.State{}, "openai", "orbit", 220, 220, time.Now(), false)
	_, _, _, a := im.At(0, 0).RGBA()
	if a != 0 {
		t.Fatal("orbit must leave corners transparent")
	}
	if im.Bounds() != image.Rect(0, 0, 220, 220) {
		t.Fatal(im.Bounds())
	}
}

func TestSharedProviderPresentation(t *testing.T) {
	for _, id := range []string{data.OpenAI, data.Claude} {
		for _, tc := range []struct {
			name, connection string
			stale            bool
			sessions         []data.Session
			status, compact  string
		}{
			{name: "connected without separate activity feed", connection: "connected", status: "CONNECTED / IDLE", compact: "Connected"},
			{name: "offline", status: "NOT CONNECTED", compact: "Offline"},
			{name: "stale", connection: "connected", stale: true, status: "LAST KNOWN / STALE", compact: "Stale"},
			{name: "working takes priority", connection: "connected", sessions: []data.Session{{Provider: id, Activity: "waiting"}, {Provider: id, Activity: "running"}}, status: "WORKING", compact: "Working"},
			{name: "waiting", connection: "connected", sessions: []data.Session{{Provider: id, Activity: "waiting"}}, status: "NEEDS YOUR INPUT", compact: "Waiting"},
			{name: "stale session ignored", connection: "connected", sessions: []data.Session{{Provider: id, Activity: "running", Stale: true}}, status: "CONNECTED / IDLE", compact: "Connected"},
			{name: "stale working", connection: "connected", stale: true, sessions: []data.Session{{Provider: id, Activity: "running"}}, status: "STALE / RUNNING", compact: "Stale"},
			{name: "sign in overrides activity", connection: "reauthentication", sessions: []data.Session{{Provider: id, Activity: "running"}}, status: "SIGN IN NEEDED", compact: "Sign in"},
		} {
			t.Run(id+"/"+tc.name, func(t *testing.T) {
				s := data.State{Providers: []data.Provider{{ID: id, Connection: tc.connection, Stale: tc.stale}}, Sessions: tc.sessions}
				got := presentProvider(s, id)
				if got.status != tc.status || compactProviderStatus(got.status) != tc.compact {
					t.Fatalf("status=%q compact=%q", got.status, compactProviderStatus(got.status))
				}
			})
		}
	}
}

func TestOrbitUsesExistingUsageFallback(t *testing.T) {
	week, short := int64(10080), int64(300)
	used, shortUsed := 42.0, 17.0
	tokens := int64(123456)
	// Account's short-only quota falls back to local weekly usage, just as the card does.
	p := data.Provider{ID: data.Claude, Connection: "connected", Quotas: []data.Quota{{WindowMinutes: &short, UsedPercent: &shortUsed}}, LocalQuotas: []data.Quota{{WindowMinutes: &week, UsedPercent: &used}}, LocalWeeklyTokens: &tokens}
	s := data.State{Providers: []data.Provider{p}}
	v := presentProvider(s, data.Claude)
	if !v.localQuota || !v.localTokens || v.quota == nil || *v.quota.UsedPercent != 42 || v.tokens != p.LocalWeeklyTokens {
		t.Fatal("existing local usage fallback changed", v)
	}
	now := time.Now()
	local := orbitBytes(DrawWidgetDetail(s, data.Claude, "orbit", 220, 220, now, false))
	s.Providers[0].LocalQuotas = nil
	s.Providers[0].LocalWeeklyTokens = nil
	if bytes.Equal(local, orbitBytes(DrawWidgetDetail(s, data.Claude, "orbit", 220, 220, now, false))) {
		t.Fatal("orbit did not render existing local usage")
	}
	s.Providers[0] = p
	s.Providers[0].Connection = "reauthentication"
	v = presentProvider(s, data.Claude)
	if v.quota != nil || len(v.quotas) != 0 || v.tokens != nil {
		t.Fatal("sign in must hide cached usage")
	}
}

func TestCircularStatusUsesConnectedProviderWithoutNewObservers(t *testing.T) {
	now := time.Now()
	for _, detail := range []string{"orbit", "strip"} {
		for _, id := range []string{data.OpenAI, data.Claude} {
			s := data.State{Providers: []data.Provider{{ID: id, Connection: "connected"}}}
			connected := orbitBytes(DrawWidgetDetail(s, id, detail, 220, 220, now, false))
			s.Sessions = []data.Session{{Provider: id, Activity: "unknown", Stale: true}}
			if !bytes.Equal(connected, orbitBytes(DrawWidgetDetail(s, id, detail, 220, 220, now, false))) {
				t.Fatal("old unknown session overrides working provider", id, detail)
			}
			s.Sessions = nil
			s.Providers[0].Connection = ""
			if bytes.Equal(connected, orbitBytes(DrawWidgetDetail(s, id, detail, 220, 220, now, false))) {
				t.Fatal("connection not reflected by circular renderer", id, detail)
			}
		}
	}
}

func TestOrbitPreservesPartialTokenQualifier(t *testing.T) {
	tokens := int64(12345)
	now := time.Now()
	s := data.State{Providers: []data.Provider{{ID: data.OpenAI, Connection: "connected", WeeklyTokens: &tokens}}}
	complete := orbitBytes(DrawWidgetDetail(s, data.OpenAI, "orbit", 220, 220, now, false))
	s.Providers[0].Partial = true
	if bytes.Equal(complete, orbitBytes(DrawWidgetDetail(s, data.OpenAI, "orbit", 220, 220, now, false))) {
		t.Fatal("partial usage must remain labelled in circular layout")
	}
}

func TestStripShowsUsageWithExistingNativeLogin(t *testing.T) {
	week, short := int64(10080), int64(300)
	used, zero := 42.0, 0.0
	tokens := int64(12345)
	for _, id := range []string{data.OpenAI, data.Claude} {
		for _, tc := range []struct {
			name      string
			provider  data.Provider
			qualifier string
			value     string
		}{
			{name: "weekly quota", provider: data.Provider{Connection: "connected", Quotas: []data.Quota{{WindowMinutes: &week, UsedPercent: &used}}}, value: "7D 42% USED"},
			{name: "zero is available", provider: data.Provider{Connection: "connected", Quotas: []data.Quota{{WindowMinutes: &week, UsedPercent: &zero}}}, value: "7D 0% USED"},
			{name: "short window fallback", provider: data.Provider{Connection: "connected", Quotas: []data.Quota{{WindowMinutes: &short, UsedPercent: &used}}}, value: "5H 42% USED"},
			{name: "token fallback", provider: data.Provider{Connection: "connected", WeeklyTokens: &tokens}, value: "12.3K TOK / WK"},
			{name: "partial tokens", provider: data.Provider{Connection: "connected", WeeklyTokens: &tokens, Partial: true}, qualifier: " / PARTIAL", value: "12.3K TOK / WK"},
			{name: "local quota", provider: data.Provider{Connection: "connected", LocalQuotas: []data.Quota{{WindowMinutes: &week, UsedPercent: &used}}}, qualifier: " / LOCAL", value: "7D 42% USED"},
			{name: "signed in without usage", provider: data.Provider{Connection: "connected"}, value: "USAGE --"},
			{name: "expired login hides cached usage", provider: data.Provider{Connection: "reauthentication", WeeklyTokens: &tokens, Quotas: []data.Quota{{WindowMinutes: &week, UsedPercent: &used}}}, value: "Sign in"},
			{name: "stale hides cached usage", provider: data.Provider{Connection: "connected", Stale: true, WeeklyTokens: &tokens}, value: "Stale"},
			{name: "disconnected hides cached usage", provider: data.Provider{Connection: "disconnected", WeeklyTokens: &tokens}, value: "Offline"},
		} {
			t.Run(id+"/"+tc.name, func(t *testing.T) {
				tc.provider.ID = id
				s := data.State{Providers: []data.Provider{tc.provider}}
				qualifier, value := stripUsage(presentProvider(s, id))
				if qualifier != tc.qualifier || value != tc.value {
					t.Fatalf("got %q %q; want %q %q", qualifier, value, tc.qualifier, tc.value)
				}
			})
		}
	}
}

func TestStripFrameUpdatesWhenQuotaChanges(t *testing.T) {
	week := int64(10080)
	used := 4.0
	s := data.State{Providers: []data.Provider{{ID: data.OpenAI, Connection: "connected", Quotas: []data.Quota{{WindowMinutes: &week, UsedPercent: &used}}}}}
	now := time.Now()
	for _, size := range []image.Point{{X: 144, Y: 27}, {X: 110, Y: 27}, {X: 150, Y: 37}} {
		used = 4
		before := orbitBytes(DrawWidgetDetail(s, data.OpenAI, "strip", size.X, size.Y, now, false))
		used = 53
		after := orbitBytes(DrawWidgetDetail(s, data.OpenAI, "strip", size.X, size.Y, now, false))
		if bytes.Equal(before, after) {
			t.Fatalf("%v strip ignores live quota changes", size)
		}
	}
}
