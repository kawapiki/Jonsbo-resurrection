//go:build windows

package main

import (
	"context"
	"github.com/kawapiki/Jonsbo-resurrection/internal/engine"
	"github.com/kawapiki/Jonsbo-resurrection/pkg/module"
	"image"
	"time"
)

type layoutRenderer interface {
	ValidateView(string) error
	Frame(context.Context, string) (image.Image, error)
}

func (s configuredSource) FarewellFrame(ctx context.Context, id, view string, elapsed time.Duration) (image.Image, error) {
	if id == "configurator" {
		if renderer, ok := s.editor.(interface {
			FarewellFrame(context.Context, string, time.Duration) (image.Image, error)
		}); ok {
			return renderer.FarewellFrame(ctx, view, elapsed)
		}
	}
	return nil, module.ErrUnsupported
}

// configuredSource keeps dynamic saved layouts outside the fixed module
// descriptor registry while preserving the same output and API contracts.
type configuredSource struct {
	*engine.Runtime
	editor layoutRenderer
}

func (s configuredSource) ValidateView(id, view string) error {
	if id == "configurator" && s.editor != nil {
		return s.editor.ValidateView(view)
	}
	return s.Runtime.ValidateView(id, view)
}
func (s configuredSource) Frame(ctx context.Context, id, view string) (image.Image, error) {
	if id == "configurator" && s.editor != nil {
		return s.editor.Frame(ctx, view)
	}
	return s.Runtime.Frame(ctx, id, view)
}
