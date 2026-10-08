//go:build windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/kawapiki/Jonsbo-resurrection/internal/config"
	"github.com/kawapiki/Jonsbo-resurrection/internal/engine"
	"github.com/kawapiki/Jonsbo-resurrection/modules/aisubscriptions"
	"github.com/kawapiki/Jonsbo-resurrection/modules/example"
	"github.com/kawapiki/Jonsbo-resurrection/modules/hardware"
	"github.com/kawapiki/Jonsbo-resurrection/pkg/module"
	"io"
	"path/filepath"
	"time"
)

// Community modules register one factory here. The engine and API do not need
// to know their concrete types, sensor sources, event payloads, or renderers.
type moduleFactory func(config.Config, json.RawMessage) (module.Module, error)

var moduleFactories = map[string]moduleFactory{
	"hardware": func(_ config.Config, options json.RawMessage) (module.Module, error) {
		d, e := moduleInterval(options)
		if e != nil {
			return nil, e
		}
		return hardware.New(d)
	},
	"example": func(_ config.Config, options json.RawMessage) (module.Module, error) {
		d, e := moduleInterval(options)
		if e != nil {
			return nil, e
		}
		return example.New(d)
	},
	"ai-subscriptions": func(c config.Config, options json.RawMessage) (module.Module, error) {
		dir, err := aiDirectory(c, options)
		if err != nil {
			return nil, err
		}
		return aisubscriptions.New(dir)
	},
}

func aiDirectory(c config.Config, raw json.RawMessage) (string, error) {
	opts := struct {
		Directory string `json:"storage_dir"`
	}{Directory: filepath.Join(filepath.Dir(c.TokenFile), "ai-subscriptions")}
	if len(raw) > 0 {
		d := json.NewDecoder(bytes.NewReader(raw))
		d.DisallowUnknownFields()
		if err := d.Decode(&opts); err != nil {
			return "", err
		}
		if err := d.Decode(new(any)); err != io.EOF {
			return "", fmt.Errorf("trailing AI module options")
		}
	}
	if opts.Directory == "" {
		return "", fmt.Errorf("AI storage_dir cannot be empty")
	}
	return filepath.Abs(opts.Directory)
}

func moduleInterval(raw json.RawMessage) (time.Duration, error) {
	opts := struct {
		Interval string `json:"interval"`
	}{Interval: "1s"}
	if len(raw) > 0 {
		d := json.NewDecoder(bytes.NewReader(raw))
		d.DisallowUnknownFields()
		if e := d.Decode(&opts); e != nil {
			return 0, e
		}
		if e := d.Decode(new(any)); e != io.EOF {
			return 0, fmt.Errorf("trailing module options")
		}
	}
	interval, e := time.ParseDuration(opts.Interval)
	if e != nil || interval < 50*time.Millisecond || interval > time.Minute {
		return 0, fmt.Errorf("module interval must be 50ms..1m")
	}
	return interval, nil
}
func newRuntime(c config.Config) (*engine.Runtime, error) {
	r := engine.New()
	for id, settings := range c.Modules {
		factory, ok := moduleFactories[id]
		if !ok {
			return nil, fmt.Errorf("unknown module %q; register its factory first", id)
		}
		if !settings.Enabled {
			continue
		}
		m, e := factory(c, settings.Options)
		if e != nil {
			return nil, fmt.Errorf("%s: %w", id, e)
		}
		if e = r.Register(m); e != nil {
			return nil, e
		}
	}
	return r, nil
}
func closeRuntime(r *engine.Runtime) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r.Close(ctx)
}
func awaitModule(ctx context.Context, r *engine.Runtime, id string) (module.State, error) {
	updates, unsubscribe := r.Subscribe()
	defer unsubscribe()
	for {
		select {
		case <-ctx.Done():
			return module.State{}, ctx.Err()
		case states := <-updates:
			found := false
			for _, s := range states {
				if s.Module.ID != id {
					continue
				}
				found = true
				if s.Status == "failed" || s.Status == "stopped" {
					return s, fmt.Errorf("%s module %s: %s", id, s.Status, s.Error)
				}
				if !s.UpdatedAt.IsZero() {
					return s, nil
				}
			}
			if !found {
				return module.State{}, module.ErrNotFound
			}
		}
	}
}
