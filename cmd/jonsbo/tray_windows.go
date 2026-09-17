//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"syscall"
	"time"

	startupconfig "github.com/kawapiki/Jonsbo-resurrection/internal/startup"
	trayui "github.com/kawapiki/Jonsbo-resurrection/internal/tray"
)

const trayName = "Local\\JonsboResurrectionTray"
const releasesURL = "https://github.com/kawapiki/Jonsbo-resurrection/releases"

// trayChild is owned exclusively by the tray's message-loop thread.
type trayChild struct {
	exe, dir, control string
	cmd               *exec.Cmd
	done              chan error
	log               *os.File
	job               syscall.Handle
	status            string
	retries           int
	retryAt           time.Time
	wanted            bool
}

func (c *trayChild) start() error {
	if c.cmd != nil {
		return nil
	}
	c.wanted = true
	c.retryAt = time.Time{}
	f, err := os.OpenFile(filepath.Join(c.dir, "server.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	fmt.Fprintf(f, "\n[%s] Starting tray-owned server\n", time.Now().Format(time.RFC3339))
	cmd := exec.Command(c.exe, trayui.ServerArgs(c.dir, c.control)...)
	cmd.Dir = c.dir
	cmd.Stdout = f
	cmd.Stderr = f
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err = cmd.Start(); err != nil {
		f.Close()
		c.status = "Could not start; open logs"
		return err
	}
	// The job closes on tray termination, so even an abrupt tray crash cannot
	// leave its server running indefinitely.
	if err = trayui.AssignChild(c.job, cmd.Process.Pid); err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		f.Close()
		return err
	}
	c.cmd = cmd
	c.log = f
	c.done = make(chan error, 1)
	c.status = "Server running (see logs for display status)"
	go func() { c.done <- cmd.Wait() }()
	return nil
}

func (c *trayChild) finish(err error) {
	if c.log != nil {
		fmt.Fprintf(c.log, "[%s] Server exited: %v\n", time.Now().Format(time.RFC3339), err)
		c.log.Close()
	}
	c.cmd = nil
	c.log = nil
	c.done = nil
	c.status = "Server stopped; open logs"
	if c.wanted {
		if d := trayui.RetryDelay(c.retries); d > 0 {
			c.retries++
			c.retryAt = time.Now().Add(d)
			c.status = "Server exited; retry pending (open logs)"
		} else {
			c.status = "Server failed; open logs or choose Start"
		}
	}
}

func (c *trayChild) tick() {
	if c.cmd != nil {
		select {
		case err := <-c.done:
			c.finish(err)
		default:
		}
	}
	if c.wanted && !c.retryAt.IsZero() && !time.Now().Before(c.retryAt) {
		if err := c.start(); err != nil {
			c.finish(err)
		}
	}
}

func (c *trayChild) stop() error {
	c.wanted = false
	c.retryAt = time.Time{}
	c.status = "Stopped"
	if c.cmd == nil {
		return nil
	}
	select {
	case err := <-c.done:
		c.finish(err)
		return nil
	default:
	}
	// Only our unique stop event is ever signalled; never stop a standalone API.
	_ = signalMonitorStop(c.control)
	timer := time.NewTimer(8 * time.Second)
	defer timer.Stop()
	select {
	case err := <-c.done:
		c.finish(err)
		return nil
	case <-timer.C:
	}
	err := c.cmd.Process.Kill()
	if err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	c.finish(<-c.done)
	return nil
}

func tray(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("tray takes no arguments")
	}
	ctx, release, err := monitorControl(context.Background(), trayName)
	if err != nil {
		return err
	}
	defer release()
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	base := os.Getenv("LOCALAPPDATA")
	if base == "" || !filepath.IsAbs(base) {
		return fmt.Errorf("LOCALAPPDATA must be an absolute directory")
	}
	dir := filepath.Join(base, "JonsboResurrection")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	job, err := trayui.NewChildJob()
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(job)
	child := &trayChild{exe: exe, dir: dir, control: fmt.Sprintf("Local\\JonsboResurrectionChild-%d", os.Getpid()), job: job, status: "Stopped"}
	defer child.stop()
	if err = child.start(); err != nil {
		child.finish(err)
		trayui.ShowError(err)
	}
	elevateStartup := func(args string) error {
		owner, err := user.Current()
		if err != nil {
			return fmt.Errorf("identify startup owner: %w", err)
		}
		return trayui.RunElevated(exe, args+" --expected-user-sid "+syscall.EscapeArg(owner.Uid))
	}
	startupAction := func(elevated bool) error {
		err := startupconfig.Enable(exe, elevated)
		if errors.Is(err, startupconfig.ErrElevationRequired) {
			args := "startup enable"
			if elevated {
				args += " --elevated"
			}
			return elevateStartup(args)
		}
		return err
	}
	// PowerShell/Task Scheduler queries must never stall the native message loop.
	type startupResult struct {
		mode startupconfig.Mode
		err  error
	}
	startupDone := make(chan startupResult, 1)
	startupMode := startupconfig.Disabled
	startupErr := fmt.Errorf("startup settings are loading")
	startupBusy := false
	nextStartupRead := time.Time{}
	readStartup := func(action func() error) error {
		if startupBusy {
			return nil
		}
		startupBusy = true
		go func() {
			if action != nil {
				trayui.ShowError(action())
			}
			m, e := startupconfig.State()
			startupDone <- startupResult{m, e}
		}()
		return nil
	}
	readStartup(nil)
	return trayui.Run(ctx, trayui.Options{
		Status: func() string { return child.status }, Tick: func() {
			child.tick()
			select {
			case result := <-startupDone:
				startupMode, startupErr = result.mode, result.err
				startupBusy = false
				nextStartupRead = time.Now().Add(15 * time.Second)
			default:
			}
			if !startupBusy && time.Now().After(nextStartupRead) {
				readStartup(nil)
			}
		},
		Start: func() error { child.retries = 0; return child.start() }, Stop: child.stop,
		OpenLogs:     func() error { return trayui.Open(filepath.Join(dir, "server.log")) },
		OpenReleases: func() error { return trayui.Open(releasesURL) },
		StartupState: func() (bool, bool, error) {
			if startupBusy {
				return startupMode != startupconfig.Disabled, startupMode == startupconfig.Elevated, fmt.Errorf("startup settings are updating")
			}
			return startupMode != startupconfig.Disabled, startupMode == startupconfig.Elevated, startupErr
		},
		ToggleStartup: func() error {
			return readStartup(func() error {
				m, e := startupconfig.State()
				if e != nil {
					return e
				}
				if m == startupconfig.Disabled {
					return startupAction(false)
				}
				e = startupconfig.Disable()
				if errors.Is(e, startupconfig.ErrElevationRequired) {
					return elevateStartup("startup disable")
				}
				return e
			})
		},
		ElevatedStartup: func() error { return readStartup(func() error { return startupAction(true) }) },
	})
}
