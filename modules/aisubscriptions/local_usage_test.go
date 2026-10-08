package aisubscriptions

import (
	data "github.com/kawapiki/Jonsbo-resurrection/pkg/aisubscriptions"
	"testing"
	"time"
)

func TestClaudeLocalFeedIsVisibleWithoutClaimingAccountTotals(t *testing.T) {
	m, dir, now := fixture(t)
	a := account(now, "claude-account")
	a.Provider = data.Claude
	a.Source = "claude-auth"
	apply(t, m, a)
	pct, minutes := 42.0, int64(10080)
	q := data.Observation{Kind: "quota", Provider: data.Claude, Source: "claude-statusline", SessionID: "s", ObservedAt: now, Quotas: []data.Quota{{ID: "seven_day", UsedPercent: &pct, WindowMinutes: &minutes, ObservedAt: now}}}
	apply(t, m, q)
	u := data.Observation{Kind: "usage", Provider: data.Claude, Source: "claude-otel", SessionID: "s", RequestID: "req1", ObservedAt: now, Tokens: &data.TokenUsage{Total: 100}}
	apply(t, m, u)
	apply(t, m, u)
	p := m.Snapshot().Providers[1]
	if p.WeeklyTokens != nil || len(p.Quotas) != 0 {
		t.Fatal("local feed was attributed to account")
	}
	if len(p.LocalQuotas) != 1 || p.LocalWeeklyTokens == nil || *p.LocalWeeklyTokens != 100 {
		t.Fatalf("local data lost: %+v", p)
	}
	m, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	m.now = func() time.Time { return now }
	apply(t, m, u)
	if *m.Snapshot().Providers[1].LocalWeeklyTokens != 100 {
		t.Fatal("restart replay inflated local total")
	}
	u.RequestID = "api-request"
	u.AuthMode = "api"
	apply(t, m, u)
	if *m.Snapshot().Providers[1].LocalWeeklyTokens != 100 {
		t.Fatal("API usage included in local fallback")
	}
	m.now = func() time.Time { return now.Add(181 * time.Second) }
	if len(m.Snapshot().Providers[1].LocalQuotas) != 0 {
		t.Fatal("expired local quota remains current")
	}
}

func TestClaudeLocalTokensRespectMondayAcrossDST(t *testing.T) {
	m, _, _ := fixture(t)
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return now }
	for i, at := range []time.Time{time.Date(2026, 10, 25, 22, 59, 0, 0, time.UTC), time.Date(2026, 10, 25, 23, 0, 0, 0, time.UTC)} {
		u := usage(at, "", "s", string(rune('a'+i)), int64(10+i))
		u.Provider = data.Claude
		u.Source = "claude-otel"
		apply(t, m, u)
	}
	if got := m.Snapshot().Providers[1].LocalWeeklyTokens; got == nil || *got != 11 {
		t.Fatalf("wrong local week: %v", got)
	}
}
