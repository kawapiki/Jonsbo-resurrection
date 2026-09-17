package telemetry

import "testing"

func TestRyzenTemperatureDecode(t *testing.T) {
	for _, tc := range []struct {
		raw   uint32
		want  float64
		valid bool
	}{
		{uint32(440) << 21, 55, true},
		{(uint32(832) << 21) | (1 << 19), 55, true},
		{(uint32(832) << 21) | (3 << 16), 55, true},
		{0, 0, false}, {0xffffffff, 0, false}, {uint32(1600) << 21, 0, false},
	} {
		got, err := decodeRyzenTemperature(tc.raw)
		if !tc.valid {
			if err == nil {
				t.Fatal("invalid raw sensor accepted")
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Fatalf("raw %x: got %v,%v", tc.raw, got, err)
		}
	}
}
