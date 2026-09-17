//go:build windows

package telemetry

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"syscall"
	"unsafe"
)

type GPU struct {
	Name             string   `json:"name"`
	UsagePercent     *float64 `json:"usage_percent"`
	TemperatureC     *float64 `json:"temperature_c"`
	HotspotC         *float64 `json:"hotspot_c"`
	ClockMHz         *float64 `json:"clock_mhz"`
	PowerW           *float64 `json:"power_w"`
	FanRPM           *float64 `json:"fan_rpm"`
	MemoryTotalBytes *uint64  `json:"memory_total_bytes"`
	MemoryUsedBytes  *uint64  `json:"memory_used_bytes"`
}

func setAMDMemory(g *GPU, total int64, totalOK bool, usedMB int32, usedOK bool) {
	g.MemoryTotalBytes, g.MemoryUsedBytes = nil, nil
	if totalOK && total >= 0 {
		v := uint64(total)
		g.MemoryTotalBytes = &v
	}
	// ADL reports dedicated usage in MB (Windows binary megabytes); widen
	// before multiplying so cards with more than 2 GiB cannot overflow int32.
	if usedOK && usedMB >= 0 {
		v := uint64(usedMB) * 1024 * 1024
		g.MemoryUsedBytes = &v
	}
}

// ADLMemoryInfo2 from AMD adl_structures.h. iMemorySize is already bytes.
type amdMemoryInfo struct {
	Size                                                   int64
	Type                                                   [256]byte
	Bandwidth, HyperMemory, InvisibleMemory, VisibleMemory int64
}
type amdSensor struct{ Supported, Value int32 }
type amdPMLog struct {
	Size    int32
	Sensors [256]amdSensor
}

// ABI and units: AMD's official include/adl_structures.h and adl_defines.h,
// https://github.com/GPUOpen-LibrariesAndSDKs/display-library/tree/master/include
func decodeAMD(name string, p *amdPMLog) GPU {
	value := func(id int, max int32) *float64 {
		s := p.Sensors[id]
		if s.Supported == 0 || s.Value < 0 || s.Value > max {
			return nil
		}
		v := float64(s.Value)
		return &v
	}
	// RDNA3 exposes board power (73) instead of the older ASIC-only value (23).
	power := value(73, 5000)
	if power == nil {
		power = value(23, 5000)
	}
	return GPU{Name: name, UsagePercent: value(19, 100), TemperatureC: value(8, 200), HotspotC: value(27, 200), ClockMHz: value(1, 20000), PowerW: power, FanRPM: value(14, 50000)}
}

type amdAdapterInfo struct {
	Size, Index                    int32
	UDID                           [256]byte
	Bus, Device, Function, Vendor  int32
	Name, DisplayName              [256]byte
	Present, Exist                 int32
	DriverPath, DriverPathExt, PNP [256]byte
	OSDisplayIndex                 int32
}
type amdAdapter struct {
	index    int32
	name     string
	discrete bool
}

// AMD performs read-only ADL queries. Calls are serialized and do not retry or
// start goroutines. ADL itself exposes no cancellation/timeout; callers needing
// a hard deadline must isolate the native collector in a helper process.
type AMD struct {
	mu                      sync.Mutex
	context                 uintptr
	dll                     *syscall.DLL
	destroy, query          *syscall.Proc
	memoryInfo, memoryUsage *syscall.Proc
	adapters                []amdAdapter
}

var amdRuntimeOnce sync.Once
var amdRuntimeErr error
var amdMalloc *syscall.Proc

func amdSystemDirectory() (string, error) {
	// kernel32 is a Windows KnownDLL; resolve the directory through the OS,
	// never through environment variables or the current working directory.
	p := syscall.NewLazyDLL("kernel32.dll").NewProc("GetSystemDirectoryW")
	if err := p.Find(); err != nil {
		return "", err
	}
	buf := make([]uint16, 32768)
	n, _, err := p.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return "", fmt.Errorf("GetSystemDirectoryW: %w", err)
	}
	if n >= uintptr(len(buf)) {
		return "", errors.New("Windows system directory exceeds buffer")
	}
	dir := syscall.UTF16ToString(buf[:n])
	if !filepath.IsAbs(dir) {
		return "", errors.New("Windows returned a relative system directory")
	}
	return dir, nil
}

func amdInitRuntime(dir string) error {
	amdRuntimeOnce.Do(func() {
		var dll *syscall.DLL
		dll, amdRuntimeErr = syscall.LoadDLL(filepath.Join(dir, "msvcrt.dll"))
		if amdRuntimeErr != nil {
			return
		}
		amdMalloc, amdRuntimeErr = dll.FindProc("malloc")
		if amdRuntimeErr != nil {
			dll.Release()
		}
		// The CRT remains loaded for the process-global allocation callback.
	})
	return amdRuntimeErr
}

// Fixed-size APIs below never return callback-allocated buffers to the caller.
// Use the C allocator as prescribed by ADL's sample; ADL owns internal storage.
var amdAllocate = syscall.NewCallback(func(size uintptr) uintptr {
	if size == 0 || size > 16<<20 {
		return 0
	}
	p, _, _ := amdMalloc.Call(size)
	return p
})

// Pointer-bearing uintptr arguments must remain pinned through this wrapper.
//
//go:uintptrescapes
func amdCall(p *syscall.Proc, args ...uintptr) error {
	r, _, _ := p.Call(args...)
	if int32(r) != 0 {
		return fmt.Errorf("%s: ADL status %d", p.Name, int32(r))
	}
	return nil
}

func NewAMD() (*AMD, error) {
	if runtime.GOARCH != "amd64" {
		return nil, errors.New("AMD collector requires Windows amd64")
	}
	dir, err := amdSystemDirectory()
	if err != nil {
		return nil, err
	}
	if err = amdInitRuntime(dir); err != nil {
		return nil, err
	}
	dll, err := syscall.LoadDLL(filepath.Join(dir, "atiadlxx.dll"))
	if err != nil {
		return nil, fmt.Errorf("load AMD ADL: %w", err)
	}
	a := &AMD{dll: dll}
	ok := false
	defer func() {
		if !ok {
			a.Close()
		}
	}()
	names := []string{"ADL2_Main_Control_Create", "ADL2_Main_Control_Destroy", "ADL2_Adapter_NumberOfAdapters_Get", "ADL2_Adapter_AdapterInfo_Get", "ADL2_New_QueryPMLogData_Get"}
	procs := make([]*syscall.Proc, len(names))
	for i, name := range names {
		procs[i], err = dll.FindProc(name)
		if err != nil {
			return nil, err
		}
	}
	a.destroy, a.query = procs[1], procs[4]
	// MemoryInfo2 is the currently documented ADL2 memory-info API. Older
	// drivers may only export the original layout (same initial fields).
	a.memoryInfo, _ = dll.FindProc("ADL2_Adapter_MemoryInfo2_Get")
	if a.memoryInfo == nil {
		a.memoryInfo, _ = dll.FindProc("ADL2_Adapter_MemoryInfo_Get")
	}
	a.memoryUsage, _ = dll.FindProc("ADL2_Adapter_DedicatedVRAMUsage_Get")
	if err = amdCall(procs[0], amdAllocate, 1, uintptr(unsafe.Pointer(&a.context))); err != nil {
		return nil, err
	}
	var count int32
	if err = amdCall(procs[2], a.context, uintptr(unsafe.Pointer(&count))); err != nil {
		return nil, err
	}
	if count <= 0 || count > 250 {
		return nil, fmt.Errorf("invalid AMD adapter count %d", count)
	}
	infos := make([]amdAdapterInfo, count)
	for i := range infos {
		infos[i].Size = int32(unsafe.Sizeof(amdAdapterInfo{}))
	}
	if err = amdCall(procs[3], a.context, uintptr(unsafe.Pointer(&infos[0])), uintptr(len(infos))*unsafe.Sizeof(infos[0])); err != nil {
		return nil, err
	}
	family, _ := dll.FindProc("ADL2_Adapter_ASICFamilyType_Get")
	seen := map[string]bool{}
	for _, info := range infos {
		// ADL historically reports AMD's vendor as decimal 1002 rather than 0x1002.
		if (info.Vendor != 1002 && info.Vendor != 0x1002) || info.Exist == 0 {
			continue
		}
		key := fmt.Sprintf("%d:%d:%d", info.Bus, info.Device, info.Function)
		if info.Bus < 0 {
			key = string(bytes.TrimRight(info.UDID[:], "\x00"))
		}
		if key == "" {
			key = fmt.Sprintf("index:%d", info.Index)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		adapter := amdAdapter{index: info.Index, name: string(bytes.TrimRight(info.Name[:], "\x00"))}
		if family != nil {
			var kind, valid int32
			if amdCall(family, a.context, uintptr(info.Index), uintptr(unsafe.Pointer(&kind)), uintptr(unsafe.Pointer(&valid))) == nil {
				adapter.discrete = kind&valid&1 != 0
			}
		}
		a.adapters = append(a.adapters, adapter)
	}
	if len(a.adapters) == 0 {
		return nil, errors.New("no physical AMD adapters found")
	}
	sort.SliceStable(a.adapters, func(i, j int) bool { return a.adapters[i].discrete && !a.adapters[j].discrete })
	runtime.KeepAlive(infos)
	ok = true
	return a, nil
}

func (a *AMD) Sample() ([]GPU, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.context == 0 {
		return nil, errors.New("AMD collector is closed")
	}
	result := make([]GPU, 0, len(a.adapters))
	var errs []error
	for _, adapter := range a.adapters {
		p := amdPMLog{Size: int32(unsafe.Sizeof(amdPMLog{}))}
		g := GPU{Name: adapter.name}
		if err := amdCall(a.query, a.context, uintptr(adapter.index), uintptr(unsafe.Pointer(&p))); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", adapter.name, err))
		} else {
			g = decodeAMD(adapter.name, &p)
		}
		mem := amdMemoryInfo{Size: -1}
		usedMB := int32(-1)
		totalOK, usedOK := false, false
		if a.memoryInfo != nil {
			err := amdCall(a.memoryInfo, a.context, uintptr(adapter.index), uintptr(unsafe.Pointer(&mem)))
			totalOK = err == nil
		}
		if a.memoryUsage != nil {
			err := amdCall(a.memoryUsage, a.context, uintptr(adapter.index), uintptr(unsafe.Pointer(&usedMB)))
			usedOK = err == nil
		}
		setAMDMemory(&g, mem.Size, totalOK, usedMB, usedOK)
		result = append(result, g)
		if g.UsagePercent == nil && g.TemperatureC == nil && g.HotspotC == nil && g.ClockMHz == nil && g.PowerW == nil && g.FanRPM == nil && g.MemoryTotalBytes == nil && g.MemoryUsedBytes == nil {
			errs = append(errs, fmt.Errorf("%s: no supported GPU sensors", adapter.name))
		}
	}
	return result, errors.Join(errs...)
}

func (a *AMD) Close() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.context != 0 && a.destroy != nil {
		a.destroy.Call(a.context)
		a.context = 0
	}
	if a.dll != nil {
		a.dll.Release()
		a.dll = nil
	}
}
