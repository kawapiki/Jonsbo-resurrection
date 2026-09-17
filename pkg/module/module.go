// Package module defines the extension contract shared by built-in and community modules.
// Modules are trusted Go code compiled into the application. They must honor context
// cancellation and return fresh, finite JSON values. No USB or platform API is required.
package module

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"regexp"
	"time"
)

const ContractVersion = 1
const MaxStateBytes = 256 << 10
const MaxEventBytes = 64 << 10

var (
	ErrNotFound    = errors.New("not found")
	ErrUnavailable = errors.New("module unavailable")
	ErrUnsupported = errors.New("operation unsupported")
	ErrInvalid     = errors.New("invalid input")
)

type View struct {
	ID     string `json:"id"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}
type Descriptor struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
	Views       []View `json:"views,omitempty"`
}
type Publish func(any) error
type Module interface {
	Descriptor() Descriptor
	Run(context.Context, Publish) error
}
type Renderer interface {
	Render(context.Context, string) (image.Image, error)
}
type Event struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}
type EventHandler interface {
	HandleEvent(context.Context, Event) error
}
type State struct {
	Module    Descriptor      `json:"module"`
	Status    string          `json:"status"`
	UpdatedAt time.Time       `json:"updated_at"`
	Data      json.RawMessage `json:"data"`
	Error     string          `json:"error,omitempty"`
}

// Assignment maps a module's logical view onto a physical display. Rotation is clockwise.
type Assignment struct {
	Module   string `json:"module"`
	View     string `json:"view"`
	Rotation int    `json:"rotation"`
}
type Display struct {
	Serial      string     `json:"serial"`
	Kind        string     `json:"kind"`
	Assignment  Assignment `json:"assignment"`
	Status      string     `json:"status"`
	LastSuccess time.Time  `json:"last_success"`
	Error       string     `json:"error,omitempty"`
}

var identifier = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
var eventType = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)

func ValidID(s string) bool { return identifier.MatchString(s) }
func Validate(d Descriptor) error {
	if !ValidID(d.ID) || d.Name == "" || len(d.Name) > 128 || d.Version == "" || len(d.Version) > 64 || len(d.Description) > 2048 || len(d.Views) > 32 {
		return fmt.Errorf("%w: module descriptor", ErrInvalid)
	}
	seen := map[string]bool{}
	for _, v := range d.Views {
		if !ValidID(v.ID) || seen[v.ID] || v.Width < 1 || v.Height < 1 || v.Width > 2048 || v.Height > 2048 || v.Width*v.Height > 2_000_000 {
			return fmt.Errorf("%w: view descriptor", ErrInvalid)
		}
		seen[v.ID] = true
	}
	return nil
}
func ValidateEvent(e Event) error {
	if !eventType.MatchString(e.Type) || len(e.Payload) > MaxEventBytes || len(e.Payload) == 0 || !json.Valid(e.Payload) {
		return fmt.Errorf("%w: event type or payload", ErrInvalid)
	}
	return nil
}
