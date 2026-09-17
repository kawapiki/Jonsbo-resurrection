package telemetry

import "testing"

func TestCPUUsage(t *testing.T) {
	for _, tc := range []struct {
		name          string
		before, after Times
		want          float64
		valid         bool
	}{
		{"busy", Times{100, 200, 100}, Times{140, 280, 120}, 60, true},
		{"idle", Times{100, 200, 100}, Times{200, 300, 100}, 0, true},
		{"first", Times{}, Times{100, 200, 100}, 0, false},
		{"reset", Times{100, 200, 100}, Times{10, 20, 10}, 0, false},
		{"no interval", Times{100, 200, 100}, Times{100, 200, 100}, 0, false},
		{"invalid idle", Times{100, 200, 100}, Times{300, 210, 100}, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := CPUUsage(tc.before, tc.after)
			if !tc.valid {
				if got != nil {
					t.Fatalf("want unavailable, got %v", *got)
				}
				return
			}
			if got == nil || *got != tc.want {
				t.Fatalf("want %v, got %v", tc.want, got)
			}
		})
	}
}
func TestMemory(t *testing.T) {
	if m := MemoryFrom(16<<30, 4<<30); m.UsedBytes != 12<<30 || m.UsagePercent == nil || *m.UsagePercent != 75 {
		t.Fatalf("incorrect memory: %+v", m)
	}
	for _, pair := range [][2]uint64{{0, 0}, {10, 20}} {
		if m := MemoryFrom(pair[0], pair[1]); m.UsagePercent != nil {
			t.Fatal("invalid counters must be unavailable")
		}
	}
}
