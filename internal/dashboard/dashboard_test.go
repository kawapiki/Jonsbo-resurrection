package dashboard

import (
	"github.com/kawapiki/Jonsbo-resurrection/internal/telemetry"
	"testing"
)

func TestUnavailableAndZero(t *testing.T) {
	zero := 0.0
	if metric(nil, "%") != "--" {
		t.Fatal("unavailable must not be shown as zero")
	}
	if metric(&zero, "%") != "0%" {
		t.Fatal("a measured zero is valid")
	}
}
func TestRenderDimensions(t *testing.T) {
	for _, tc := range []struct {
		role string
		h    int
	}{{"summary", 480}, {"cpu", 180}, {"gpu", 180}, {"memory", 180}} {
		im, err := Render(telemetry.Snapshot{}, tc.role)
		if err != nil {
			t.Fatal(err)
		}
		if im.Bounds().Dx() != 640 || im.Bounds().Dy() != tc.h {
			t.Fatalf("%s wrong dimensions", tc.role)
		}
		if _, _, _, a := im.At(0, 0).RGBA(); a != 65535 {
			t.Fatal("dashboard must be opaque")
		}
	}
	if _, err := Render(telemetry.Snapshot{}, "typo"); err == nil {
		t.Fatal("unknown role accepted")
	}
}
func TestMemoryUnavailable(t *testing.T) {
	used, total := memoryLabels(telemetry.Memory{})
	if used != "--" || total != "--" {
		t.Fatalf("missing RAM fabricated capacities: %s / %s", used, total)
	}
	m := telemetry.MemoryFrom(16<<30, 4<<30)
	used, total = memoryLabels(m)
	if used != "12.0" || total != "16.0" {
		t.Fatalf("wrong GiB: %s / %s", used, total)
	}
}
func TestVRAMLabels(t *testing.T) {
	used, total := vramLabels(telemetry.GPU{})
	if used != "--" || total != "--" {
		t.Fatal("unavailable memory fabricated")
	}
	u, v := uint64(3<<29), uint64(24<<30)
	used, total = vramLabels(telemetry.GPU{MemoryUsedBytes: &u, MemoryTotalBytes: &v})
	if used != "1.5" || total != "24.0" {
		t.Fatalf("bad GiB: %s/%s", used, total)
	}
}
