package configurator

import (
	"context"
	"testing"
	"time"
)

// Keep a repeatable CPU/allocation measurement for the native circular presets.
func BenchmarkCircularRender(b *testing.B) {
	now := time.Unix(1800000000, 0)
	value := 52.0
	m := &Module{source: &fakeSource{}, metrics: []Metric{{ID: "cpu.temp", Unit: "C", Value: &value}, {ID: "gpu.temp", Unit: "C", Value: &value}, {ID: "cpu.usage", Unit: "%", Value: &value}}}
	for i := 0; i < 120; i++ {
		m.history = append(m.history, History{Time: now.Add(time.Duration(i-119) * time.Second), Values: map[string]*float64{"cpu.usage": &value}})
	}
	for _, theme := range circularThemes() {
		if theme.ID != "cyclops-console" && theme.ID != "cyclops-openai" {
			continue
		}
		b.Run(theme.ID, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := m.render(context.Background(), theme.Layout, now); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
