//go:build windows

package main

import (
	"context"
	"errors"
	"github.com/kawapiki/Jonsbo-resurrection/internal/engine"
	"github.com/kawapiki/Jonsbo-resurrection/pkg/module"
	"image"
	"testing"
)

type testLayouts struct{}

func (testLayouts) ValidateView(id string) error {
	if id != "saved-layout" {
		return module.ErrNotFound
	}
	return nil
}
func (testLayouts) Frame(ctx context.Context, id string) (image.Image, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if id != "saved-layout" {
		return nil, module.ErrNotFound
	}
	return image.NewRGBA(image.Rect(0, 0, 640, 180)), nil
}
func TestDynamicConfiguratorRouting(t *testing.T) {
	s := configuredSource{Runtime: engine.New(), editor: testLayouts{}}
	if e := s.ValidateView("configurator", "saved-layout"); e != nil {
		t.Fatal(e)
	}
	if !errors.Is(s.ValidateView("other", "saved-layout"), module.ErrNotFound) {
		t.Fatal("layout leaked into other module")
	}
	img, e := s.Frame(context.Background(), "configurator", "saved-layout")
	if e != nil || img.Bounds().Dy() != 180 {
		t.Fatal("dynamic frame unavailable", e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = s.Frame(ctx, "configurator", "saved-layout"); !errors.Is(e, context.Canceled) {
		t.Fatal("cancel ignored")
	}
}
