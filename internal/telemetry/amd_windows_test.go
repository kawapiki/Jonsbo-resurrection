//go:build windows

package telemetry

import (
	"encoding/json"
	"os"
	"testing"
	"unsafe"
)

// Catches treating unsupported or stale values as real zero, wrong sensor IDs,
// and incorrect unit conversion. Values/IDs follow AMD's published PMLog enum.
func TestAMDDecodeSupportedSensors(t *testing.T) {
	var p amdPMLog
	p.Sensors[1] = amdSensor{1, 2310}
	p.Sensors[8] = amdSensor{1, 47}
	p.Sensors[14] = amdSensor{1, 0}
	p.Sensors[19] = amdSensor{1, 23}
	p.Sensors[23] = amdSensor{1, 115}
	p.Sensors[27] = amdSensor{0, 999}
	g := decodeAMD("GPU", &p)
	if g.ClockMHz == nil || *g.ClockMHz != 2310 || g.TemperatureC == nil || *g.TemperatureC != 47 || g.UsagePercent == nil || *g.UsagePercent != 23 || g.PowerW == nil || *g.PowerW != 115 || g.FanRPM == nil || *g.FanRPM != 0 || g.HotspotC != nil {
		t.Fatalf("wrong sensor mapping: %+v", g)
	}
	fresh := decodeAMD("GPU", &amdPMLog{})
	if fresh.UsagePercent != nil || fresh.TemperatureC != nil || fresh.ClockMHz != nil || fresh.FanRPM != nil || fresh.PowerW != nil {
		t.Fatal("missing sensors must be null")
	}
}

func TestAMDRejectImpossibleReadings(t *testing.T) {
	var p amdPMLog
	p.Sensors[19] = amdSensor{1, 101}
	p.Sensors[8] = amdSensor{1, -1}
	g := decodeAMD("GPU", &p)
	if g.UsagePercent != nil || g.TemperatureC != nil {
		t.Fatal("invalid readings accepted")
	}
}

func TestAMDPrefersBoardPowerWhenSupported(t *testing.T) {
	var p amdPMLog
	p.Sensors[73] = amdSensor{1, 174}
	p.Sensors[23] = amdSensor{1, 120}
	g := decodeAMD("GPU", &p)
	if g.PowerW == nil || *g.PowerW != 174 {
		t.Fatal("board power should take precedence over ASIC-only power")
	}
}

func TestAMDLive(t *testing.T) {
	if os.Getenv("JONSBO_AMD_LIVE_TEST") != "1" {
		t.Skip("opt-in native AMD probe")
	}
	a, err := NewAMD()
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	g, err := a.Sample()
	if err != nil {
		t.Logf("partial sample: %v", err)
	}
	if len(g) == 0 {
		t.Fatal("no GPU results")
	}
	b, _ := json.Marshal(g)
	t.Log(string(b))
	a.Close()
	if _, err := a.Sample(); err == nil {
		t.Fatal("closed collector accepted sample")
	}
}

func TestAMDSystemDirectoryIgnoresEnvironment(t *testing.T) {
	want, err := amdSystemDirectory()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SystemRoot", `Z:\untrusted`)
	t.Setenv("WINDIR", `Z:\untrusted`)
	got, err := amdSystemDirectory()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("system directory changed with environment: %q -> %q", want, got)
	}
}

func TestAMDMemoryUnitsAndMissingSamples(t *testing.T) {
	g := GPU{}
	setAMDMemory(&g, 25769803776, true, 1536, true)
	if g.MemoryTotalBytes == nil || *g.MemoryTotalBytes != 25769803776 || g.MemoryUsedBytes == nil || *g.MemoryUsedBytes != 1610612736 {
		t.Fatalf("bad VRAM units: %+v", g)
	}
	setAMDMemory(&g, 0, false, 0, false)
	if g.MemoryTotalBytes != nil || g.MemoryUsedBytes != nil {
		t.Fatal("unavailable query reused previous memory value")
	}
	setAMDMemory(&g, 25769803776, true, 0, true)
	if g.MemoryUsedBytes == nil || *g.MemoryUsedBytes != 0 {
		t.Fatal("successful zero usage was discarded")
	}
	setAMDMemory(&g, -1, true, -1, true)
	if g.MemoryTotalBytes != nil || g.MemoryUsedBytes != nil {
		t.Fatal("negative driver sentinel accepted")
	}
}

func TestAMDCPUTemperatureProbe(t *testing.T) {
	if os.Getenv("JONSBO_AMD_LIVE_TEST") != "1" {
		t.Skip("opt-in native AMD sensor32 probe")
	}
	a, err := NewAMD()
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	for _, adapter := range a.adapters {
		p := amdPMLog{Size: int32(unsafe.Sizeof(amdPMLog{}))}
		err := amdCall(a.query, a.context, uintptr(adapter.index), uintptr(unsafe.Pointer(&p)))
		t.Logf("%s: PMLog CPU sensor32 supported=%d value=%d error=%v", adapter.name, p.Sensors[32].Supported, p.Sensors[32].Value, err)
	}
}
