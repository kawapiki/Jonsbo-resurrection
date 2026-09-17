// Package api exposes the authenticated, loopback-only module API.
package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kawapiki/Jonsbo-resurrection/pkg/module"
)

const maxBody = module.MaxEventBytes + 1024

type Backend interface {
	Snapshot() []module.State
	Subscribe() (<-chan []module.State, func())
	Frame(context.Context, string, string) (image.Image, error)
	Event(context.Context, string, module.Event) error
}

type Displays interface {
	Displays() []module.Display
	Assign(string, module.Assignment) error
}

// ValidateListen requires a literal loopback address and an explicit numeric port.
func ValidateListen(address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil || !loopback(host) || !validPort(port) {
		return errors.New("listen address must be a loopback IP and numeric port (0–65535)")
	}
	return nil
}

func validPort(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	n, err := strconv.Atoi(s)
	return err == nil && n >= 0 && n <= 65535
}

func loopback(s string) bool {
	ip := net.ParseIP(s)
	return ip != nil && ip.IsLoopback() && (strings.Contains(s, ".") && !strings.Contains(s, ":") || s == "::1")
}

func validHost(s string) bool {
	if s == "[::1]" {
		return true
	}
	host := s
	if strings.Contains(s, ":") {
		var port string
		var err error
		host, port, err = net.SplitHostPort(s)
		if err != nil || !validPort(port) {
			return false
		}
	}
	return host == "localhost" || loopback(host)
}

type handler struct {
	backend  Backend
	displays Displays
	token    [32]byte
	renders  chan struct{}
	streams  chan struct{}
	requests chan struct{}
}

// NewHandler leaves listener, HTTP server timeouts, and token storage to the caller.
func NewHandler(backend Backend, displays Displays, token string) (http.Handler, error) {
	if backend == nil {
		return nil, errors.New("backend is required")
	}
	if len(token) < 32 || strings.TrimSpace(token) != token || strings.ContainsAny(token, "\r\n") {
		return nil, errors.New("token must contain at least 32 characters without surrounding whitespace")
	}
	return &handler{backend: backend, displays: displays, token: sha256.Sum256([]byte(token)), renders: make(chan struct{}, 2), streams: make(chan struct{}, 32), requests: make(chan struct{}, 32)}, nil
}

func fail(w http.ResponseWriter, status int) { http.Error(w, http.StatusText(status), status) }
func backendError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, module.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, module.ErrInvalid):
		status = http.StatusBadRequest
	case errors.Is(err, module.ErrUnavailable), errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		status = http.StatusServiceUnavailable
	case errors.Is(err, module.ErrUnsupported):
		status = http.StatusMethodNotAllowed
	}
	fail(w, status)
}
func method(w http.ResponseWriter, r *http.Request, want string) bool {
	if r.Method == want {
		return true
	}
	w.Header().Set("Allow", want)
	fail(w, http.StatusMethodNotAllowed)
	return false
}
func sendJSON(w http.ResponseWriter, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		fail(w, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}
func decode(w http.ResponseWriter, r *http.Request, value any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	defer r.Body.Close()
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	err := d.Decode(value)
	if err == nil {
		var extra any
		err = d.Decode(&extra)
		if err == io.EOF {
			return true
		}
	}
	var limit *http.MaxBytesError
	if errors.As(err, &limit) {
		fail(w, http.StatusRequestEntityTooLarge)
	} else {
		fail(w, http.StatusBadRequest)
	}
	return false
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	auth := r.Header.Values("Authorization")
	provided := ""
	if len(auth) == 1 && strings.HasPrefix(auth[0], "Bearer ") {
		provided = strings.TrimPrefix(auth[0], "Bearer ")
	}
	sum := sha256.Sum256([]byte(provided))
	if subtle.ConstantTimeCompare(h.token[:], sum[:]) != 1 {
		w.Header().Set("WWW-Authenticate", "Bearer")
		fail(w, http.StatusUnauthorized)
		return
	}
	if !validHost(r.Host) {
		fail(w, http.StatusForbidden)
		return
	}
	origins := r.Header.Values("Origin")
	if len(origins) > 1 || len(origins) == 1 && origins[0] != "http://"+r.Host {
		fail(w, http.StatusForbidden)
		return
	}
	if r.URL.Path == "/v1/events" {
		if method(w, r, http.MethodGet) {
			h.events(w, r)
		}
		return
	}
	select {
	case h.requests <- struct{}{}:
		defer func() { <-h.requests }()
	default:
		fail(w, http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Now().Add(5 * time.Second))
	_ = controller.SetWriteDeadline(time.Now().Add(5 * time.Second))
	defer controller.SetReadDeadline(time.Time{})
	defer controller.SetWriteDeadline(time.Time{})
	switch r.URL.Path {
	case "/v1/modules":
		if !method(w, r, http.MethodGet) {
			return
		}
		out := []module.Descriptor{}
		for _, state := range h.backend.Snapshot() {
			out = append(out, state.Module)
		}
		sendJSON(w, out)
		return
	case "/v1/state":
		if !method(w, r, http.MethodGet) {
			return
		}
		sendJSON(w, nonNil(h.backend.Snapshot()))
		return
	case "/v1/displays":
		if !method(w, r, http.MethodGet) {
			return
		}
		out := []module.Display{}
		if h.displays != nil {
			out = h.displays.Displays()
			if out == nil {
				out = []module.Display{}
			}
		}
		sendJSON(w, out)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(parts) >= 3 && parts[0] == "v1" && parts[1] == "modules" {
		id := parts[2]
		if !module.ValidID(id) {
			fail(w, 400)
			return
		}
		if len(parts) == 3 {
			if !method(w, r, http.MethodGet) {
				return
			}
			for _, state := range h.backend.Snapshot() {
				if state.Module.ID == id {
					sendJSON(w, state)
					return
				}
			}
			fail(w, 404)
			return
		}
		if len(parts) == 4 && parts[3] == "events" {
			if !method(w, r, http.MethodPost) {
				return
			}
			var event module.Event
			if !decode(w, r, &event) {
				return
			}
			if err := module.ValidateEvent(event); err != nil {
				backendError(w, err)
				return
			}
			if err := h.backend.Event(ctx, id, event); err != nil {
				backendError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if len(parts) == 5 && parts[3] == "views" && strings.HasSuffix(parts[4], ".png") {
			if !method(w, r, http.MethodGet) {
				return
			}
			view := strings.TrimSuffix(parts[4], ".png")
			if !module.ValidID(view) {
				fail(w, 400)
				return
			}
			h.frame(w, r, id, view)
			return
		}
	}
	if len(parts) == 4 && parts[0] == "v1" && parts[1] == "displays" && parts[3] == "assignment" {
		if !method(w, r, http.MethodPut) {
			return
		}
		if !validSerial(parts[2]) {
			fail(w, 400)
			return
		}
		if h.displays == nil {
			backendError(w, module.ErrUnsupported)
			return
		}
		var assignment module.Assignment
		if !decode(w, r, &assignment) {
			return
		}
		if !module.ValidID(assignment.Module) || !module.ValidID(assignment.View) || assignment.Rotation != 0 && assignment.Rotation != 90 && assignment.Rotation != 180 && assignment.Rotation != 270 {
			fail(w, 400)
			return
		}
		if err := h.displays.Assign(parts[2], assignment); err != nil {
			backendError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	fail(w, 404)
}

func validSerial(s string) bool {
	if len(s) == 0 || len(s) > 256 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}
func nonNil(states []module.State) []module.State {
	if states == nil {
		return []module.State{}
	}
	return states
}

func (h *handler) frame(w http.ResponseWriter, r *http.Request, id, view string) {
	select {
	case h.renders <- struct{}{}:
		defer func() { <-h.renders }()
	default:
		fail(w, 503)
		return
	}
	img, err := h.backend.Frame(r.Context(), id, view)
	if err != nil {
		backendError(w, err)
		return
	}
	if img == nil {
		fail(w, 500)
		return
	}
	bounds := img.Bounds()
	x, y := bounds.Dx(), bounds.Dy()
	if x < 1 || y < 1 || x > 2_000_000 || y > 2_000_000 || x > 2_000_000/y {
		fail(w, 500)
		return
	}
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		fail(w, 500)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Write(data.Bytes())
}

func (h *handler) events(w http.ResponseWriter, r *http.Request) {
	select {
	case h.streams <- struct{}{}:
		defer func() { <-h.streams }()
	default:
		fail(w, 503)
		return
	}
	updates, unsubscribe := h.backend.Subscribe()
	defer unsubscribe()
	controller := http.NewResponseController(w)
	defer controller.SetWriteDeadline(time.Time{})
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	write := func(states []module.State, heartbeat bool) bool {
		if err := controller.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
			return false
		}
		if heartbeat {
			if _, err := io.WriteString(w, ": heartbeat\n\n"); err != nil {
				return false
			}
		} else {
			data, err := json.Marshal(nonNil(states))
			if err != nil {
				return false
			}
			if _, err := fmt.Fprintf(w, "event: state\ndata: %s\n\n", data); err != nil {
				return false
			}
		}
		return controller.Flush() == nil
	}
	// Runtimes may enqueue the initial snapshot atomically with subscription.
	// Consume that snapshot first so a newer Snapshot call cannot precede an
	// older queued update. Backends without an initial update remain supported.
	var initial []module.State
	select {
	case states, ok := <-updates:
		if !ok {
			return
		}
		initial = states
	default:
		initial = h.backend.Snapshot()
	}
	if !write(initial, false) {
		return
	}
	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case states, ok := <-updates:
			if !ok || !write(states, false) {
				return
			}
		case <-tick.C:
			if !write(nil, true) {
				return
			}
		}
	}
}
