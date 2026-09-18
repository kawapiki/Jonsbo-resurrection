//go:build windows

package main

import (
	"context"
	"github.com/kawapiki/Jonsbo-resurrection/internal/engine"
	"image"
)

type layoutRenderer interface {
	ValidateView(string) error
	Frame(context.Context, string) (image.Image, error)
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
