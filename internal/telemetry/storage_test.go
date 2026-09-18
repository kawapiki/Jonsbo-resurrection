package telemetry

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

func TestDiskUsageBounds(t *testing.T) {
	for _, tc := range []struct {
		total, free uint64
		want        float64
		valid       bool
	}{
		{100, 25, 75, true}, {100, 100, 0, true}, {100, 0, 100, true},
		{0, 0, 0, false}, {10, 11, 0, false}, {math.MaxUint64, 0, 100, true},
	} {
		d := DiskFrom(`C:\`, tc.total, tc.free)
		if (d.UsagePercent != nil) != tc.valid {
			t.Fatalf("%+v: validity mismatch: %+v", tc, d)
		}
		if tc.valid && math.Abs(*d.UsagePercent-tc.want) > 0.00001 {
			t.Fatalf("%+v: got %v", tc, *d.UsagePercent)
		}
		if tc.valid && (d.TotalBytes != tc.total || d.FreeBytes != tc.free || d.Path != `C:\`) {
			t.Fatalf("capacity or path changed: %+v", d)
		}
		if !tc.valid && (d.TotalBytes != 0 || d.FreeBytes != 0 || d.Path != `C:\`) {
			t.Fatalf("invalid reading: %+v", d)
		}
	}
}

func TestStorageFiltersOrdersCachesAndClones(t *testing.T) {
	calls := 0
	fail := false
	s := storageSampler{
		systemDrive: "d:",
		logicalDrives: func() (uint32, error) {
			calls++
			if fail {
				return 0, errors.New("enumeration unavailable")
			}
			return 0x7f, nil
		},
		driveType: func(p string) uint32 {
			return map[byte]uint32{'A': 2, 'B': 0, 'C': 3, 'D': 3, 'E': 4, 'F': 5, 'G': 6}[p[0]]
		},
		diskSpace: func(p string) (uint64, uint64, error) {
			if p == `C:\` {
				return 0, 0, errors.New("access denied")
			}
			return 100, 25, nil
		},
	}
	now := time.Now()
	disks, warnings := s.sample(now)
	if len(disks) != 2 || disks[0].Path != `D:\` || disks[1].Path != `C:\` {
		t.Fatalf("fixed/system order: %+v", disks)
	}
	if disks[1].UsagePercent != nil || len(warnings) != 1 || !strings.Contains(warnings[0], "access denied") {
		t.Fatalf("failure missing: %+v %v", disks, warnings)
	}
	*disks[0].UsagePercent = 0
	disks[0].Path = "corrupted"
	warnings[0] = "corrupted"
	disks, warnings = s.sample(now.Add(9 * time.Second))
	if calls != 1 || disks[0].Path != `D:\` || *disks[0].UsagePercent != 75 || !strings.Contains(warnings[0], "access denied") {
		t.Fatalf("cache mutated or resampled: %+v %v calls=%d", disks, warnings, calls)
	}
	fail = true
	disks, warnings = s.sample(now.Add(10 * time.Second))
	if calls != 2 || len(disks) != 0 || disks == nil || len(warnings) != 1 {
		t.Fatalf("failed refresh stale or null: %+v %v calls=%d", disks, warnings, calls)
	}
	b, err := json.Marshal(Snapshot{Disks: disks})
	if err != nil || !strings.Contains(string(b), `"disks":[]`) {
		t.Fatalf("JSON: %s %v", b, err)
	}
}
