//go:build windows

package tray

import (
	"fmt"
	"syscall"
	"unsafe"
)

type jobBasicLimit struct {
	ProcessTime, JobTime         int64
	Flags                        uint32
	MinWorkingSet, MaxWorkingSet uintptr
	ActiveProcesses              uint32
	Affinity                     uintptr
	Priority, Scheduling         uint32
}
type jobExtendedLimit struct {
	Basic                                                      jobBasicLimit
	IO                                                         [6]uint64
	ProcessMemory, JobMemory, PeakProcessMemory, PeakJobMemory uintptr
}

func NewChildJob() (syscall.Handle, error) {
	h, _, err := kernel.NewProc("CreateJobObjectW").Call(0, 0)
	if h == 0 {
		return 0, fmt.Errorf("create server job: %v", err)
	}
	limits := jobExtendedLimit{}
	limits.Basic.Flags = 0x2000 // JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	r, _, err := kernel.NewProc("SetInformationJobObject").Call(h, 9, uintptr(unsafe.Pointer(&limits)), unsafe.Sizeof(limits))
	if r == 0 {
		syscall.CloseHandle(syscall.Handle(h))
		return 0, fmt.Errorf("configure server job: %v", err)
	}
	return syscall.Handle(h), nil
}

func AssignChild(job syscall.Handle, pid int) error {
	h, err := syscall.OpenProcess(0x100|1, false, uint32(pid)) // SET_QUOTA | TERMINATE
	if err != nil {
		return fmt.Errorf("open server process: %w", err)
	}
	defer syscall.CloseHandle(h)
	r, _, err := kernel.NewProc("AssignProcessToJobObject").Call(uintptr(job), uintptr(h))
	if r == 0 {
		return fmt.Errorf("assign server job: %v", err)
	}
	return nil
}
