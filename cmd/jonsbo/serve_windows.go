//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/kawapiki/Jonsbo-resurrection/internal/api"
	"github.com/kawapiki/Jonsbo-resurrection/internal/config"
	"github.com/kawapiki/Jonsbo-resurrection/internal/configuratorweb"
	"github.com/kawapiki/Jonsbo-resurrection/internal/output"
	"github.com/kawapiki/Jonsbo-resurrection/internal/winusb"
	"github.com/kawapiki/Jonsbo-resurrection/modules/configurator"
	"github.com/kawapiki/Jonsbo-resurrection/pkg/module"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"
)

const serverName = "Local\\JonsboGoAPIServer"

func validChildControlName(name string) bool {
	if name == "" {
		return true
	}
	const prefix = "Local\\JonsboResurrectionChild-"
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	suffix := strings.TrimPrefix(name, prefix)
	if len(suffix) == 0 || len(suffix) > 10 {
		return false
	}
	for _, c := range suffix {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	configPath := fs.String("config", "", "local JSON configuration file")
	listen := fs.String("listen", "", "loopback IP:port (default 127.0.0.1:8787)")
	tokenFile := fs.String("token-file", "", "bearer token file (default bin/api-token)")
	all := fs.Bool("all", false, "also drive attached USB displays")
	exampleEnabled := fs.Bool("example", false, "enable the example module")
	interval := fs.Duration("interval", 0, "override hardware polling interval")
	refresh := fs.Duration("frame-interval", 0, "override display refresh interval")
	duration := fs.Duration("duration", 0, "stop after duration; 0 until interrupted")
	logPath := fs.String("log-file", "", "append startup/display status to file")
	controlName := fs.String("control-name", "", "private tray child stop control")
	allowNoDisplays := fs.Bool("allow-no-displays", false, "allow API-only operation when no displays are attached")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 || *duration < 0 || *interval < 0 || *refresh < 0 || !validChildControlName(*controlName) {
		return fmt.Errorf("invalid serve arguments")
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	if *listen != "" {
		cfg.Listen = *listen
	}
	if *tokenFile != "" {
		cfg.TokenFile = *tokenFile
	}
	if *refresh != 0 {
		cfg.FrameInterval = refresh.String()
	}
	if cfg.Modules == nil {
		cfg.Modules = map[string]config.Module{}
	}
	if *interval != 0 {
		cfg.Modules["hardware"] = config.Module{Enabled: true, Options: json.RawMessage(fmt.Sprintf(`{"interval":%q}`, interval.String()))}
	}
	if *exampleEnabled {
		settings := cfg.Modules["example"]
		settings.Enabled = true
		cfg.Modules["example"] = settings
	}
	if err = config.Validate(cfg); err != nil {
		return err
	}
	runtime, err := newRuntime(cfg)
	if err != nil {
		return err
	}
	defer closeRuntime(runtime)
	var log io.Writer = os.Stdout
	if *logPath != "" {
		f, e := os.OpenFile(*logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		defer f.Close()
		log = f
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if *duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *duration)
		defer cancel()
	}
	ctx, release, err := monitorControl(ctx, serverName)
	if err != nil {
		return err
	}
	defer release()
	token, err := config.Token(cfg.TokenFile)
	if err != nil {
		return err
	}
	editor, err := configurator.New(filepath.Join(filepath.Dir(cfg.TokenFile), "configurator"), runtime)
	if err != nil {
		return fmt.Errorf("configurator: %w", err)
	}
	if settings, ok := cfg.Modules["hardware"]; ok {
		if interval, e := moduleInterval(settings.Options); e == nil {
			editor.SetMaxSampleAge(max(3*interval, 3*time.Second))
		}
	}
	if err = runtime.Register(editor); err != nil {
		return err
	}
	source := configuredSource{Runtime: runtime, editor: editor}
	var manager *output.Manager
	var displays api.Displays
	if *all {
		ds, e := winusb.Enumerate()
		if e != nil {
			return e
		}
		if len(ds) == 0 && !*allowNoDisplays {
			return fmt.Errorf("no supported displays connected")
		}
		assignments := map[string]module.Assignment{}
		// Restore saved editor bindings only for attached displays. Retain missing
		// device bindings on disk for the next time those displays are connected.
		for serial, a := range editor.Bindings() {
			for _, d := range ds {
				if strings.EqualFold(serial, d.Serial) {
					assignments[strings.ToUpper(serial)] = a
					break
				}
			}
		}
		configuredSerials := map[string]bool{}
		for serial, a := range cfg.Displays {
			key := strings.ToUpper(serial)
			if configuredSerials[key] {
				return fmt.Errorf("duplicate configured display %s", key)
			}
			configuredSerials[key] = true
			if _, saved := assignments[key]; !saved {
				assignments[key] = a
			}
		}
		if len(ds) == 0 {
			fmt.Fprintln(log, "No displays attached; API only. Restart monitoring after connecting displays.")
		} else {
			manager, e = output.New(source, output.USBDriver{}, ds, output.Options{Assignments: assignments, OnChange: statusLogger(log)})
			if e != nil {
				return e
			}
			displays = manager
		}
	}
	handler, err := api.NewHandlerWithUI(source, displays, token, editor.Handler(displays), configuratorweb.Files())
	if err != nil {
		return err
	}
	if *controlName != "" {
		var childRelease func()
		ctx, childRelease, err = monitorControl(ctx, *controlName)
		if err != nil {
			return err
		}
		defer childRelease()
	}
	if *all {
		var unlock func()
		ctx, unlock, err = monitorControl(ctx, monitorName)
		if err != nil {
			return err
		}
		defer unlock()
	}
	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return err
	}
	defer listener.Close()
	if err = runtime.Start(ctx); err != nil {
		return err
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10, BaseContext: func(net.Listener) context.Context { return ctx }}
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Serve(listener) }()
	displayDone := make(chan error, 1)
	if manager != nil {
		rate, _ := time.ParseDuration(cfg.FrameInterval)
		go func() { displayDone <- manager.Run(ctx, rate) }()
	}
	fmt.Fprintf(log, "API ready at http://%s/v1 (token file: %s; contract %d)\n", listener.Addr(), cfg.TokenFile, module.ContractVersion)
	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-serverDone:
		stop()
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = server.Shutdown(shutdown); err != nil {
		server.Close()
	}
	stop()
	if manager != nil {
		select {
		case e := <-displayDone:
			if e != nil && serveErr == nil {
				serveErr = e
			}
		case <-shutdown.Done():
			if serveErr == nil {
				serveErr = shutdown.Err()
			}
		}
	}
	if errors.Is(serveErr, http.ErrServerClosed) {
		return nil
	}
	return serveErr
}
