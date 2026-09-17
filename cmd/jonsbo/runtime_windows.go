//go:build windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"github.com/kawapiki/Jonsbo-resurrection/internal/config"
	"github.com/kawapiki/Jonsbo-resurrection/internal/engine"
	"github.com/kawapiki/Jonsbo-resurrection/modules/example"
	"github.com/kawapiki/Jonsbo-resurrection/modules/hardware"
	"github.com/kawapiki/Jonsbo-resurrection/pkg/module"
	"time"
)

// Community modules register one factory here. The engine and API do not need
// to know their concrete types, sensor sources, event payloads, or renderers.
type moduleFactory func(json.RawMessage) (module.Module, error)

var moduleFactories = map[string]moduleFactory{
	"hardware": func(options json.RawMessage) (module.Module, error) {
		d, e := moduleInterval(options)
		if e != nil {
			return nil, e
		}
		return hardware.New(d)
	},
	"example": func(options json.RawMessage) (module.Module, error) {
		d, e := moduleInterval(options)
		if e != nil {
			return nil, e
		}
		return example.New(d)
	},
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
		m, e := factory(settings.Options)
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
