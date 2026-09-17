//go:build windows

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"github.com/kawapiki/Jonsbo-resurrection/internal/config"
	"github.com/kawapiki/Jonsbo-resurrection/internal/device"
	"github.com/kawapiki/Jonsbo-resurrection/internal/output"
	"github.com/kawapiki/Jonsbo-resurrection/internal/winusb"
	"github.com/kawapiki/Jonsbo-resurrection/pkg/module"
	"os"
	"os/signal"
	"time"
)

func stats(args []string) error {
	fs := flag.NewFlagSet("stats", flag.ContinueOnError)
	count := fs.Int("count", 1, "JSON samples, 0 means continuous")
	interval := fs.Duration("interval", time.Second, "sampling interval")
	path := fs.String("output", "", "write JSON lines to a file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 || *count < 0 || *interval < 500*time.Millisecond || *interval > time.Minute {
		return fmt.Errorf("stats requires count >= 0 and interval 500ms..1m; no positional arguments")
	}
	c := config.Default()
	c.Modules["hardware"] = config.Module{Enabled: true, Options: json.RawMessage(fmt.Sprintf(`{"interval":%q}`, interval.String()))}
	r, err := newRuntime(c)
	if err != nil {
		return err
	}
	defer closeRuntime(r)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	var out io.Writer = os.Stdout
	if *path != "" {
		f, e := os.Create(*path)
		if e != nil {
			return e
		}
		defer f.Close()
		out = f
	}
	updates, unsubscribe := r.Subscribe()
	defer unsubscribe()
	if err = r.Start(ctx); err != nil {
		return err
	}
	enc := json.NewEncoder(out)
	var last time.Time
	sent := 0
	for {
		select {
		case <-ctx.Done():
			return nil
		case states := <-updates:
			for _, s := range states {
				if s.Module.ID != "hardware" {
					continue
				}
				if s.Status == "failed" {
					return fmt.Errorf("hardware: %s", s.Error)
				}
				if s.UpdatedAt.IsZero() || s.UpdatedAt.Equal(last) {
					continue
				}
				if err = enc.Encode(s.Data); err != nil {
					return err
				}
				last = s.UpdatedAt
				sent++
				if *count > 0 && sent >= *count {
					return nil
				}
			}
		}
	}
}
func selectDisplays(all bool, serial string) ([]device.Device, error) {
	ds, err := winusb.Enumerate()
	if err != nil {
		return nil, err
	}
	if !all {
		d, e := device.Select(ds, serial)
		if e != nil {
			return nil, e
		}
		ds = []device.Device{d}
	}
	if len(ds) == 0 {
		return nil, fmt.Errorf("no supported displays connected")
	}
	return ds, nil
}
func statusLogger(out io.Writer) func(module.Display) {
	return func(d module.Display) {
		if d.Error != "" {
			fmt.Fprintf(out, "%s %s/%s: %s\n", d.Serial, d.Assignment.Module, d.Assignment.View, d.Error)
		} else {
			fmt.Fprintf(out, "%s: %s %s/%s\n", d.Serial, d.Status, d.Assignment.Module, d.Assignment.View)
		}
	}
}
func monitor(args []string) error {
	fs := flag.NewFlagSet("monitor", flag.ContinueOnError)
	all := fs.Bool("all", false, "drive all supported displays")
	serial := fs.String("serial", "", "one display serial")
	role := fs.String("role", "", "summary, cpu, gpu, memory for a single display")
	interval := fs.Duration("interval", time.Second, "sampling and display interval")
	duration := fs.Duration("duration", 0, "stop after duration, 0 until interrupted")
	once := fs.Bool("once", false, "send one snapshot then stop")
	previews := fs.String("preview-dir", "", "save first logical frame per display")
	logPath := fs.String("log-file", "", "append status and warnings to file")
	if e := fs.Parse(args); e != nil {
		return e
	}
	if fs.NArg() != 0 || *all == (*serial != "") || *interval < 500*time.Millisecond || *interval > time.Minute || *duration < 0 {
		return fmt.Errorf("choose --all or --serial; interval 500ms..1m, duration >= 0")
	}
	if *role != "" && (*all || (*role != "summary" && *role != "cpu" && *role != "gpu" && *role != "memory")) {
		return fmt.Errorf("--role requires one display and summary, cpu, gpu or memory")
	}
	var log io.Writer = os.Stdout
	if *logPath != "" {
		f, e := os.OpenFile(*logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		defer f.Close()
		log = f
	}
	ds, e := selectDisplays(*all, *serial)
	if e != nil {
		return e
	}
	cfg := config.Default()
	cfg.Modules["hardware"] = config.Module{Enabled: true, Options: json.RawMessage(fmt.Sprintf(`{"interval":%q}`, interval.String()))}
	r, e := newRuntime(cfg)
	if e != nil {
		return e
	}
	defer closeRuntime(r)
	manager, e := output.New(r, output.USBDriver{}, ds, output.Options{PreviewDir: *previews, OnChange: statusLogger(log)})
	if e != nil {
		return e
	}
	if *role != "" {
		a := manager.Displays()[0].Assignment
		a.View = *role
		if e = manager.Assign(ds[0].Serial, a); e != nil {
			return e
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if *duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *duration)
		defer cancel()
	}
	ctx, release, e := monitorControl(ctx, monitorName)
	if e != nil {
		return e
	}
	defer release()
	if e = r.Start(ctx); e != nil {
		return e
	}
	first, e := awaitModule(ctx, r, "hardware")
	if ctx.Err() != nil {
		return nil
	}
	if e != nil {
		return e
	}
	var warnings struct {
		Warnings []string `json:"warnings"`
	}
	json.Unmarshal(first.Data, &warnings)
	for _, w := range warnings.Warnings {
		fmt.Fprintln(log, "sensor:", w)
	}
	if *once {
		return manager.Once(ctx)
	}
	return manager.Run(ctx, *interval)
}
