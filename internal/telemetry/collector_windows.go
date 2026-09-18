//go:build windows

package telemetry

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"
)

var kernel = syscall.NewLazyDLL("kernel32.dll")
var getTimes = kernel.NewProc("GetSystemTimes")
var getMemory = kernel.NewProc("GlobalMemoryStatusEx")

type Collector struct {
	previous   Times
	amd        *AMD
	amdError   error
	ryzen      *RyzenTemperature
	ryzenError error
	storage    *storageSampler
}

func New() *Collector {
	a, e := NewAMD()
	r, re := NewRyzenTemperature()
	return &Collector{amd: a, amdError: e, ryzen: r, ryzenError: re, storage: newStorageSampler()}
}
func (c *Collector) Close() {
	if c.ryzen != nil {
		c.ryzen.Close()
	}
	if c.amd != nil {
		c.amd.Close()
	}
}
func (c *Collector) Sample() Snapshot {
	s := Snapshot{Time: time.Now(), GPUs: []GPU{}, Disks: []Disk{}}
	if c.storage == nil {
		c.storage = newStorageSampler()
	}
	s.Disks, s.Warnings = c.storage.sample(s.Time)
	var now Times
	if ok, _, e := getTimes.Call(uintptr(unsafe.Pointer(&now.Idle)), uintptr(unsafe.Pointer(&now.Kernel)), uintptr(unsafe.Pointer(&now.User))); ok == 0 {
		s.Warnings = append(s.Warnings, fmt.Sprintf("CPU usage: %v", e))
		c.previous = Times{}
	} else {
		s.CPU.UsagePercent = CPUUsage(c.previous, now)
		c.previous = now
	}
	// MEMORYSTATUSEX has two DWORDs followed by seven DWORDLONGs.
	var mem struct {
		Length, Load                                                                          uint32
		TotalPhys, AvailPhys, TotalPage, AvailPage, TotalVirtual, AvailVirtual, AvailExtended uint64
	}
	mem.Length = uint32(unsafe.Sizeof(mem))
	if ok, _, e := getMemory.Call(uintptr(unsafe.Pointer(&mem))); ok == 0 {
		s.Warnings = append(s.Warnings, fmt.Sprintf("RAM: %v", e))
	} else {
		s.Memory = MemoryFrom(mem.TotalPhys, mem.AvailPhys)
	}
	if c.amd != nil {
		var err error
		s.GPUs, err = c.amd.Sample()
		if err != nil {
			s.Warnings = append(s.Warnings, err.Error())
		}
	} else if c.amdError != nil {
		s.Warnings = append(s.Warnings, c.amdError.Error())
	}
	if c.ryzen != nil {
		t, err := c.ryzen.Sample()
		if err == nil {
			s.CPU.TemperatureC = &t
			s.CPU.TemperatureSource = "Ryzen Tctl/Tdie (PawnIO)"
		} else {
			s.Warnings = append(s.Warnings, err.Error())
		}
	} else if c.ryzenError != nil {
		s.Warnings = append(s.Warnings, c.ryzenError.Error())
	}
	return s
}
