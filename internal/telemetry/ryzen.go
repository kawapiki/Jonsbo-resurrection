package telemetry

import "fmt"

// THM_TCON_CUR_TMP: bits31:21 are eighths of a degree; newer Zen parts
// signal a 49-degree range adjustment in bit19 or TJ_SEL bits17:16.
// Register interpretation cross-checked against LibreHardwareMonitor Amd17Cpu.
func decodeRyzenTemperature(raw uint32) (float64, error) {
	if raw == 0 || raw == 0xffffffff {
		return 0, fmt.Errorf("invalid Ryzen temperature register 0x%08x", raw)
	}
	t := float64(raw>>21) * 0.125
	if raw&(1<<19) != 0 || raw&(3<<16) == 3<<16 {
		t -= 49
	}
	if t < 0 || t > 125 {
		return 0, fmt.Errorf("Ryzen temperature outside supported range: %.3f C", t)
	}
	return t, nil
}
