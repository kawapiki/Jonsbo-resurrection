package configurator

import (
	data "github.com/kawapiki/Jonsbo-resurrection/pkg/aisubscriptions"
	"testing"
	"time"
)

func TestClaudeMetricsUseExplicitLocalFallback(t *testing.T) {
	now := time.Now()
	tokens := int64(12345)
	pct := 42.0
	minutes := int64(10080)
	s := data.State{Providers: []data.Provider{{ID: "claude", Connection: "connected", AccountKey: "a", UpdatedAt: now, LocalWeeklyTokens: &tokens, LocalQuotas: []data.Quota{{ID: "seven_day", UsedPercent: &pct, WindowMinutes: &minutes, ObservedAt: now}}}}}
	values := map[string]*float64{}
	aiMetrics(s, now, values)
	if values["ai.claude.tokens"] == nil || *values["ai.claude.tokens"] != 12345 || values["ai.claude.weekly"] == nil || *values["ai.claude.weekly"] != 42 {
		t.Fatalf("local metrics absent: %+v", values)
	}
	s.Providers[0].LocalQuotas[0].ObservedAt = now.Add(-181 * time.Second)
	values = map[string]*float64{}
	aiMetrics(s, now, values)
	if values["ai.claude.weekly"] != nil {
		t.Fatal("stale local quota is charted")
	}
}
