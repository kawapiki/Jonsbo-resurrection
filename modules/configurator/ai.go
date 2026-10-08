package configurator

import (
	"encoding/json"
	data "github.com/kawapiki/Jonsbo-resurrection/pkg/aisubscriptions"
	"github.com/kawapiki/Jonsbo-resurrection/pkg/module"
	"strings"
	"time"
)

func aiState(states []module.State, now time.Time) data.State {
	for _, st := range states {
		if st.Module.ID != "ai-subscriptions" {
			continue
		}
		var state data.State
		if json.Unmarshal(st.Data, &state) != nil {
			break
		}
		for i := range state.Providers {
			p := &state.Providers[i]
			if p.UpdatedAt.IsZero() || now.Sub(p.UpdatedAt) > 180*time.Second || now.Sub(p.UpdatedAt) < -time.Second {
				p.Stale = true
			}
		}
		if st.Status != "running" || st.Error != "" || st.UpdatedAt.IsZero() || now.Sub(st.UpdatedAt) > 180*time.Second || now.Sub(st.UpdatedAt) < -time.Second {
			for i := range state.Providers {
				state.Providers[i].Stale = true
			}
			for i := range state.Sessions {
				state.Sessions[i].Stale = true
			}
		}
		return state
	}
	return data.State{Providers: []data.Provider{}, Sessions: []data.Session{}}
}

func aiMetrics(state data.State, now time.Time, values map[string]*float64) {
	put := func(id string, value float64) {
		if finite(value) && value >= 0 {
			copy := value
			values[id] = &copy
		}
	}
	for _, p := range state.Providers {
		if p.ID != "openai" && p.ID != "claude" {
			continue
		}
		if p.AccountKey == "" || p.Connection != "connected" || p.Stale || p.UpdatedAt.IsZero() || now.Sub(p.UpdatedAt) > 180*time.Second || now.Sub(p.UpdatedAt) < -time.Second {
			continue
		}
		prefix := "ai." + p.ID + "."
		quotas := p.Quotas
		quotaFreshness := 180 * time.Second
		if p.ID == "claude" {
			quotaFreshness = 5 * time.Minute
		}
		if p.ID == "claude" && len(quotas) == 0 {
			quotas = p.LocalQuotas
			quotaFreshness = 180 * time.Second
		}
		// Select the provider's seven-day window by its duration, never by order.
		for _, q := range quotas {
			if q.WindowMinutes != nil && *q.WindowMinutes == 7*24*60 && !strings.HasPrefix(q.ID, "seven_day_") && q.UsedPercent != nil && *q.UsedPercent <= 100 && !q.ObservedAt.IsZero() && now.Sub(q.ObservedAt) <= quotaFreshness && now.Sub(q.ObservedAt) >= -time.Second && (q.ResetsAt == nil || q.ResetsAt.After(now)) {
				put(prefix+"weekly", *q.UsedPercent)
				break
			}
		}
		if p.WeeklyTokens != nil {
			put(prefix+"tokens", float64(*p.WeeklyTokens))
		} else if p.ID == "claude" && p.LocalWeeklyTokens != nil {
			put(prefix+"tokens", float64(*p.LocalWeeklyTokens))
		}
		if p.Capabilities.LocalSessions || p.Capabilities.BrowserActivity {
			count := 0
			for _, s := range state.Sessions {
				if s.Provider == p.ID && s.AccountKey == p.AccountKey && !s.Stale && (s.Activity == "running" || s.Activity == "waiting") {
					count++
				}
			}
			put(prefix+"sessions", float64(count))
		}
	}
}
