//go:build windows

package main

import (
	"context"
	"fmt"
	"sync"
	"syscall"
	"unsafe"
)

const monitorName = "Local\\JonsboGoHardwareMonitor"

var controlDLL = syscall.NewLazyDLL("kernel32.dll")
var createMutex = controlDLL.NewProc("CreateMutexW")
var createEvent = controlDLL.NewProc("CreateEventW")
var openEvent = controlDLL.NewProc("OpenEventW")
var setEvent = controlDLL.NewProc("SetEvent")
var waitEvent = controlDLL.NewProc("WaitForSingleObject")

func monitorControl(parent context.Context, name string) (context.Context, func(), error) {
	n, _ := syscall.UTF16PtrFromString(name)
	mutex, _, e := createMutex.Call(0, 0, uintptr(unsafe.Pointer(n)))
	if mutex == 0 {
		return nil, nil, fmt.Errorf("monitor lock: %v", e)
	}
	if e == syscall.Errno(183) {
		syscall.CloseHandle(syscall.Handle(mutex))
		return nil, nil, fmt.Errorf("a hardware monitor is already running; use jonsbo stop first")
	}
	en, _ := syscall.UTF16PtrFromString(name + "-Stop")
	event, _, e := createEvent.Call(0, 1, 0, uintptr(unsafe.Pointer(en)))
	if event == 0 {
		syscall.CloseHandle(syscall.Handle(mutex))
		return nil, nil, fmt.Errorf("monitor stop event: %v", e)
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			r, _, _ := waitEvent.Call(event, 100)
			if r == 0 || r == 0xffffffff {
				cancel()
				return
			}
		}
	}()
	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			cancel()
			<-done
			syscall.CloseHandle(syscall.Handle(event))
			syscall.CloseHandle(syscall.Handle(mutex))
		})
	}
	return ctx, cleanup, nil
}
func signalMonitorStop(name string) error {
	n, _ := syscall.UTF16PtrFromString(name + "-Stop")
	event, _, e := openEvent.Call(2, 0, uintptr(unsafe.Pointer(n)))
	if event == 0 {
		return fmt.Errorf("no accessible running monitor: %v", e)
	}
	defer syscall.CloseHandle(syscall.Handle(event))
	if ok, _, e := setEvent.Call(event); ok == 0 {
		return fmt.Errorf("stop monitor: %v", e)
	}
	return nil
}
