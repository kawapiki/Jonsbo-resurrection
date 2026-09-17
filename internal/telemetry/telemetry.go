// Package telemetry collects host counters. Missing readings are null, never zero substitutes.
package telemetry

import "time"

type Times struct{ Idle, Kernel, User uint64 }

func CPUUsage(a, b Times) *float64 {
	if a == (Times{}) || b.Idle < a.Idle || b.Kernel < a.Kernel || b.User < a.User {
		return nil
	}
	total := (b.Kernel - a.Kernel) + (b.User - a.User)
	idle := b.Idle - a.Idle
	if total == 0 || idle > total {
		return nil
	}
	v := 100 * float64(total-idle) / float64(total)
	return &v
}

type CPU struct {
	TemperatureSource string   `json:"temperature_source,omitempty"`
	UsagePercent      *float64 `json:"usage_percent"`
	TemperatureC      *float64 `json:"temperature_c"`
}
type Memory struct {
	TotalBytes   uint64   `json:"total_bytes"`
	UsedBytes    uint64   `json:"used_bytes"`
	UsagePercent *float64 `json:"usage_percent"`
}

func MemoryFrom(total, available uint64) Memory {
	if total == 0 || available > total {
		return Memory{}
	}
	p := 100 * float64(total-available) / float64(total)
	return Memory{total, total - available, &p}
}

type Snapshot struct {
	Time     time.Time `json:"time"`
	CPU      CPU       `json:"cpu"`
	Memory   Memory    `json:"memory"`
	GPUs     []GPU     `json:"gpus"`
	Warnings []string  `json:"warnings,omitempty"`
}
