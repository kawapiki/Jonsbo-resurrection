package aisubscriptions

import (
	"testing"
	"time"

	data "github.com/kawapiki/Jonsbo-resurrection/pkg/aisubscriptions"
)

func TestClaudeAccountQuotaWorksWithoutLocalSessionAndKeepsCachedReading(t *testing.T) {
	m, dir, now := fixture(t)
	a := account(now, "claude-account")
	a.Provider = data.Claude
	a.Source = "claude-auth"
	apply(t, m, a)
	pct, minutes := 69.0, int64(10080)
	q := data.Observation{Kind: "quota", Provider: data.Claude, Source: "claude-native-usage", AccountKey: a.AccountKey, AuthMode: "subscription", ObservedAt: now, Quotas: []data.Quota{{ID: "seven_day", UsedPercent: &pct, WindowMinutes: &minutes, ObservedAt: now}}}
	apply(t, m, q)
	state := m.Snapshot()
	if len(state.Sessions) != 0 || len(state.Providers[1].Quotas) != 1 || !state.Providers[1].Capabilities.AccountQuota || state.Providers[1].WeeklyTokens != nil {
		t.Fatalf("background quota depended on activity or invented tokens: %+v", state)
	}
	reopened, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	reopened.now = func() time.Time { return now.Add(time.Hour) }
	p := reopened.Snapshot().Providers[1]
	if len(p.Quotas) != 1 || *p.Quotas[0].UsedPercent != 69 || !p.Stale {
		t.Fatal("cached quota lost or displayed as fresh")
	}
}

func TestClaudeBackgroundQuotaRemainsFreshAcrossItsPollingInterval(t *testing.T) {
	m, _, now := fixture(t)
	a := account(now, "claude-account")
	a.Provider = data.Claude
	a.Source = "claude-auth"
	apply(t, m, a)
	pct, minutes := 69.0, int64(10080)
	apply(t, m, data.Observation{Kind: "quota", Provider: data.Claude, Source: "claude-native-usage", AccountKey: a.AccountKey, AuthMode: "subscription", ObservedAt: now, Quotas: []data.Quota{{ID: "seven_day", UsedPercent: &pct, WindowMinutes: &minutes, ObservedAt: now}}})
	for _, age := range []time.Duration{241 * time.Second, 301 * time.Second} {
		at := now.Add(age)
		m.now = func() time.Time { return at }
		a.ObservedAt = at
		apply(t, m, a)
		if got := m.Snapshot().Providers[1].Stale; got != (age > 5*time.Minute) {
			t.Fatalf("quota stale=%v at %s", got, age)
		}
	}
}

func TestClaudeModelQuotaDoesNotBecomeGeneralWeeklyQuota(t *testing.T) {
	pct, minutes := 69.0, int64(10080)
	if q := weeklyQuota([]data.Quota{{ID: "seven_day_opus", UsedPercent: &pct, WindowMinutes: &minutes}}); q != nil {
		t.Fatal("Opus quota presented as general weekly quota")
	}
}
