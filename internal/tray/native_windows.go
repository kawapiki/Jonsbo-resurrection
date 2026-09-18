//go:build windows

// Package tray implements a native notification-area window without cgo.
package tray

import (
	"context"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

var user = syscall.NewLazyDLL("user32.dll")
var shell = syscall.NewLazyDLL("shell32.dll")
var kernel = syscall.NewLazyDLL("kernel32.dll")

type point struct{ X, Y int32 }
type message struct {
	Window         uintptr
	ID             uint32
	WParam, LParam uintptr
	Time           uint32
	Point          point
	Private        uint32
}
type windowClass struct {
	Size, Style                        uint32
	Proc                               uintptr
	ClassExtra, WindowExtra            int32
	Instance, Icon, Cursor, Background uintptr
	Menu, Name                         *uint16
	SmallIcon                          uintptr
}
type notifyIcon struct {
	Size                uint32
	Window              uintptr
	ID, Flags, Callback uint32
	Icon                uintptr
	Tip                 [128]uint16
	State, StateMask    uint32
	Info                [256]uint16
	Version             uint32
	InfoTitle           [64]uint16
	InfoFlags           uint32
	GUID                [16]byte
	BalloonIcon         uintptr
}

// Options callbacks run on the message-loop thread. Tick must not block.
type Options struct {
	Status                         func() string
	Tick                           func()
	Start, Stop                    func() error
	OpenLogs, OpenReleases         func() error
	OpenConfigurator               func() error
	StartupState                   func() (bool, bool, error)
	ToggleStartup, ElevatedStartup func() error
}

func wide(s string) *uint16 { p, _ := syscall.UTF16PtrFromString(s); return p }

// ShowError remains visible in a windowsgui executable with no console.
func ShowError(err error) {
	if err != nil {
		user.NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(wide(err.Error()))), uintptr(unsafe.Pointer(wide("Jonsbo Resurrection"))), 0x10)
	}
}

// Open invokes the Windows shell for a trusted file, directory, or HTTPS URL.
func Open(target string) error                { return shellExecute("open", target, "") }
func RunElevated(exe, arguments string) error { return shellExecute("runas", exe, arguments) }
func shellExecute(verb, target, args string) error {
	r, _, _ := shell.NewProc("ShellExecuteW").Call(0, uintptr(unsafe.Pointer(wide(verb))), uintptr(unsafe.Pointer(wide(target))), uintptr(unsafe.Pointer(wide(args))), 0, 1)
	if r <= 32 {
		return fmt.Errorf("Windows could not open %s (code %d)", target, r)
	}
	return nil
}

func Run(ctx context.Context, o Options) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	instance, _, _ := kernel.NewProc("GetModuleHandleW").Call(0)
	icon, _, _ := user.NewProc("LoadIconW").Call(instance, 1)
	if icon == 0 {
		icon, _, _ = user.NewProc("LoadIconW").Call(0, 32512)
	}
	className := wide("JonsboResurrectionTrayWindow")
	taskbar, _, _ := user.NewProc("RegisterWindowMessageW").Call(uintptr(unsafe.Pointer(wide("TaskbarCreated"))))
	const callback = 0x8001
	var nid notifyIcon
	iconPresent := false
	update := func(op uintptr) bool {
		if op == 1 && !iconPresent {
			op = 0
		}
		if o.Status != nil {
			nid.Tip = [128]uint16{}
			t := syscall.StringToUTF16("Jonsbo Resurrection: " + o.Status())
			if len(t) > 127 {
				t = t[:127]
			}
			copy(nid.Tip[:], t)
		}
		r, _, _ := shell.NewProc("Shell_NotifyIconW").Call(op, uintptr(unsafe.Pointer(&nid)))
		if op == 2 {
			iconPresent = false
		} else {
			iconPresent = r != 0
		}
		return r != 0
	}
	popup := func(hwnd uintptr) {
		menu, _, _ := user.NewProc("CreatePopupMenu").Call()
		if menu == 0 {
			return
		}
		defer user.NewProc("DestroyMenu").Call(menu)
		appendItem := func(id uintptr, label string, flags uintptr) {
			user.NewProc("AppendMenuW").Call(menu, flags, id, uintptr(unsafe.Pointer(wide(label))))
		}
		appendItem(0, o.Status(), 2)
		if o.OpenConfigurator != nil {
			appendItem(8, "Open configurator", 0)
		}
		appendItem(1, "Start / retry displays", 0)
		appendItem(2, "Stop displays and API", 0)
		appendItem(3, "Open logs", 0)
		appendItem(4, "Open releases", 0)
		enabled, elevated, err := o.StartupState()
		var flags uintptr
		if enabled {
			flags = 8
		}
		if err != nil {
			flags |= 2
		}
		appendItem(5, "Start with Windows", flags)
		flags = 0
		if elevated {
			flags = 8
		}
		if err != nil {
			flags |= 2
		}
		appendItem(6, "Start with Windows as administrator (CPU temperature)", flags)
		appendItem(7, "Quit", 0)
		var pt point
		user.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&pt)))
		user.NewProc("SetForegroundWindow").Call(hwnd)
		selected, _, _ := user.NewProc("TrackPopupMenu").Call(menu, 0x100|2, uintptr(pt.X), uintptr(pt.Y), 0, hwnd, 0)
		user.NewProc("PostMessageW").Call(hwnd, 0, 0, 0)
		var action func() error
		switch selected {
		case 8:
			action = o.OpenConfigurator
		case 1:
			action = o.Start
		case 2:
			action = o.Stop
		case 3:
			action = o.OpenLogs
		case 4:
			action = o.OpenReleases
		case 5:
			action = o.ToggleStartup
		case 6:
			action = o.ElevatedStartup
		case 7:
			user.NewProc("DestroyWindow").Call(hwnd)
		}
		if action != nil {
			ShowError(action())
		}
	}
	proc := syscall.NewCallback(func(hwnd uintptr, msg uint32, w, l uintptr) uintptr {
		switch {
		case msg == uint32(taskbar):
			update(0)
			return 0
		case msg == callback:
			if l == 0x205 || l == 0x202 || l == 0x7b {
				popup(hwnd)
			}
			return 0
		case msg == 0x113:
			select {
			case <-ctx.Done():
				user.NewProc("DestroyWindow").Call(hwnd)
			default:
				if o.Tick != nil {
					o.Tick()
				}
				update(1)
			}
			return 0
		case msg == 0x10:
			user.NewProc("DestroyWindow").Call(hwnd)
			return 0
		case msg == 0x11:
			return 1 // Permit Windows logoff; WM_ENDSESSION follows.
		case msg == 0x16:
			if w != 0 {
				user.NewProc("DestroyWindow").Call(hwnd)
			}
			return 0
		case msg == 2:
			update(2)
			user.NewProc("PostQuitMessage").Call(0)
			return 0
		}
		r, _, _ := user.NewProc("DefWindowProcW").Call(hwnd, uintptr(msg), w, l)
		return r
	})
	wc := windowClass{Proc: proc, Instance: instance, Icon: icon, Name: className}
	wc.Size = uint32(unsafe.Sizeof(wc))
	if r, _, e := user.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return fmt.Errorf("register tray window: %v", e)
	}
	defer user.NewProc("UnregisterClassW").Call(uintptr(unsafe.Pointer(className)), instance)
	hwnd, _, e := user.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(className)), 0, 0, 0, 0, 0, 0, 0, instance, 0)
	if hwnd == 0 {
		return fmt.Errorf("create tray window: %v", e)
	}
	defer user.NewProc("DestroyWindow").Call(hwnd)
	nid = notifyIcon{Window: hwnd, ID: 1, Flags: 1 | 2 | 4, Callback: callback, Icon: icon}
	nid.Size = uint32(unsafe.Sizeof(nid))
	// Explorer may not yet exist during login. Its TaskbarCreated broadcast adds it.
	update(0)
	defer update(2)
	if timer, _, e := user.NewProc("SetTimer").Call(hwnd, 1, 1000, 0); timer == 0 {
		return fmt.Errorf("tray timer: %v", e)
	}
	defer user.NewProc("KillTimer").Call(hwnd, 1)
	var m message
	for {
		r, _, e := user.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) == -1 {
			return fmt.Errorf("tray messages: %v", e)
		}
		if r == 0 {
			return nil
		}
		user.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&m)))
		user.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&m)))
	}
}
