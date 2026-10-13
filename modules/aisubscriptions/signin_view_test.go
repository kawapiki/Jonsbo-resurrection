package aisubscriptions

import (
	"bytes"
	"image"
	"testing"
	"time"

	data "github.com/kawapiki/Jonsbo-resurrection/pkg/aisubscriptions"
)

func TestSignInRequiredReplacesCachedUsageAndStopsMascot(t *testing.T) {
	now := time.Unix(1720000000, 0)
	for _, provider := range []string{data.OpenAI, data.Claude} {
		for _, size := range []image.Point{{608, 164}, {360, 180}, {480, 480}, {640, 300}} {
			pct, tokens, minutes := 72.0, int64(1250000), int64(10080)
			s := data.State{Providers: []data.Provider{{ID: provider, Connection: "reauthentication", Stale: true, Quotas: []data.Quota{{UsedPercent: &pct, WindowMinutes: &minutes}}, WeeklyTokens: &tokens}}, Sessions: []data.Session{{Provider: provider, Activity: "running"}}}
			first := DrawWidgetDetail(s, provider, "compact", size.X, size.Y, now, true).(*image.RGBA)
			pct, tokens = 13, 9200000
			second := DrawWidgetDetail(s, provider, "compact", size.X, size.Y, now.Add(time.Second), true).(*image.RGBA)
			if !bytes.Equal(first.Pix, second.Pix) {
				t.Fatalf("%s %v sign-in view still presents changing cached usage or animates", provider, size)
			}
			// The main message must have a visible warning treatment in the centre,
			// rather than only a small status label above stale usage values.
			warning := 0
			for y := 0; y < size.Y; y++ {
				for x := 16; x < size.X-16; x++ {
					c := first.RGBAAt(x, y)
					if c.R > 200 && c.G > 140 && c.B < 120 {
						warning++
					}
				}
			}
			if warning < 250 {
				t.Fatalf("%s %v lacks a prominent sign-in warning: %d pixels", provider, size, warning)
			}
			s.Providers[0].Connection = "connected"
			s.Providers[0].Stale = false
			healthy := DrawWidgetDetail(s, provider, "compact", size.X, size.Y, now, false).(*image.RGBA)
			if bytes.Equal(first.Pix, healthy.Pix) {
				t.Fatalf("%s %v did not return to usage after reconnecting", provider, size)
			}
		}
	}
}

func TestExpandedSignInViewHidesCachedSessionTokensAndContext(t *testing.T) {
	now := time.Unix(1720000000, 0)
	for _, provider := range []string{data.OpenAI, data.Claude} {
		for _, size := range []image.Point{{480, 300}, {480, 480}, {608, 380}} {
			tokens, used, limit := int64(15000), int64(75000), int64(200000)
			s := data.State{Providers: []data.Provider{{ID: provider, Connection: "reauthentication"}}, Sessions: []data.Session{{Provider: provider, Activity: "running", ConsumedTokens: &tokens, Context: &data.ContextUsage{Used: &used, Limit: &limit}}}}
			first := DrawWidgetDetail(s, provider, "expanded", size.X, size.Y, now, true).(*image.RGBA)
			tokens, used, limit = 81000, 120000, 300000
			second := DrawWidgetDetail(s, provider, "expanded", size.X, size.Y, now.Add(time.Second), true).(*image.RGBA)
			if !bytes.Equal(first.Pix, second.Pix) {
				t.Fatalf("%s %v sign-in view still shows cached session tokens or context", provider, size)
			}
		}
	}
}

func TestOneProvidersSignInWarningDoesNotReplaceOtherProvidersCard(t *testing.T) {
	now := time.Unix(1720000000, 0)
	s := data.State{Providers: []data.Provider{{ID: data.OpenAI, Connection: "connected"}, {ID: data.Claude, Connection: "connected"}}}
	before := DrawWidgetDetail(s, "overview", "compact", 640, 480, now, false).(*image.RGBA)
	s.Providers[0].Connection = "reauthentication"
	after := DrawWidgetDetail(s, "overview", "compact", 640, 480, now, false).(*image.RGBA)
	for y := 268; y < 468; y++ {
		for x := 12; x < 628; x++ {
			if before.RGBAAt(x, y) != after.RGBAAt(x, y) {
				t.Fatal("OpenAI sign-in warning changed the Claude card")
			}
		}
	}
	if bytes.Equal(before.Pix, after.Pix) {
		t.Fatal("overview did not show OpenAI sign-in failure")
	}
}

func TestNativeSignInFailureReachesDisplayBeforeAndAfterAnAccountIsLinked(t *testing.T) {
	for _, provider := range []string{data.OpenAI, data.Claude} {
		for _, hadAccount := range []bool{false, true} {
			m, _, now := fixture(t)
			source := "openai-app-server"
			if provider == data.Claude {
				source = "claude-auth"
			}
			if hadAccount {
				apply(t, m, data.Observation{Kind: "account", Provider: provider, Source: source, AccountKey: "verified", AuthMode: "subscription", Connection: "connected", ObservedAt: now})
			}
			apply(t, m, data.Observation{Kind: "account", Provider: provider, Source: source, Connection: "reauthentication", ObservedAt: now.Add(time.Second)})
			actual := DrawWidgetDetail(m.Snapshot(), provider, "compact", 608, 164, now, true).(*image.RGBA)
			expected := DrawWidgetDetail(data.State{Providers: []data.Provider{{ID: provider, Connection: "reauthentication"}}}, provider, "compact", 608, 164, now, false).(*image.RGBA)
			if !bytes.Equal(actual.Pix, expected.Pix) {
				t.Fatalf("%s linked=%t native sign-in failure did not reach the display", provider, hadAccount)
			}
		}
	}
}
