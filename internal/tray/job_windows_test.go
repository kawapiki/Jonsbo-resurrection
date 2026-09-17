//go:build windows

package tray

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func TestJobCloseTerminatesOwnedChild(t *testing.T) {
	if os.Getenv("JONSBO_TRAY_TEST_CHILD") == "1" {
		time.Sleep(time.Minute)
		return
	}
	job, err := NewChildJob()
	if err != nil {
		t.Fatal(err)
	}
	closed := false
	defer func() {
		if !closed {
			syscall.CloseHandle(job)
		}
	}()
	cmd := exec.Command(os.Args[0], "-test.run=^TestJobCloseTerminatesOwnedChild$")
	cmd.Env = append(os.Environ(), "JONSBO_TRAY_TEST_CHILD=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	if err = AssignChild(job, cmd.Process.Pid); err != nil {
		t.Fatal(err)
	}
	if err = syscall.CloseHandle(job); err != nil {
		t.Fatal(err)
	}
	closed = true
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("child survived owner job closing")
	}
}

func TestWindowsAMD64ABI(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("amd64 ABI")
	}
	if unsafe.Sizeof(notifyIcon{}) != 976 {
		t.Fatalf("NOTIFYICONDATAW size %d", unsafe.Sizeof(notifyIcon{}))
	}
	if unsafe.Sizeof(windowClass{}) != 80 {
		t.Fatalf("WNDCLASSEXW size %d", unsafe.Sizeof(windowClass{}))
	}
	if unsafe.Sizeof(jobExtendedLimit{}) != 144 {
		t.Fatalf("JOBOBJECT_EXTENDED_LIMIT_INFORMATION size %d", unsafe.Sizeof(jobExtendedLimit{}))
	}
}
