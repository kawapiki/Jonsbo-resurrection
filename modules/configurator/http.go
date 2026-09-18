package configurator

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kawapiki/Jonsbo-resurrection/pkg/module"
)

type Displays interface {
	Displays() []module.Display
	Assign(string, module.Assignment) error
}
type apiError struct {
	code    int
	message string
}

func (e *apiError) Error() string { return e.message }
func conflict(s string) error     { return &apiError{http.StatusConflict, s} }
func jsonResponse(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func body(w http.ResponseWriter, r *http.Request, v any, limit int64) error {
	return decodeStrict(http.MaxBytesReader(w, r.Body, limit), v)
}

// Handler expects the host API's authentication and loopback/Origin checks to run first.
func (m *Module) Handler(displays Displays) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		e := m.serve(w, r, displays)
		if e != nil {
			code := http.StatusInternalServerError
			var ae *apiError
			var oversized *http.MaxBytesError
			switch {
			case errors.As(e, &oversized):
				code = http.StatusRequestEntityTooLarge
			case errors.As(e, &ae):
				code = ae.code
			case errors.Is(e, module.ErrInvalid):
				code = 400
			case errors.Is(e, module.ErrNotFound):
				code = 404
			case errors.Is(e, module.ErrUnavailable):
				code = 503
			}
			jsonResponse(w, code, map[string]string{"error": e.Error()})
		}
	})
}
func (m *Module) serve(w http.ResponseWriter, r *http.Request, d Displays) error {
	const prefix = "/v1/configurator"
	if r.URL.Path != prefix && !strings.HasPrefix(r.URL.Path, prefix+"/") {
		return module.ErrNotFound
	}
	p := strings.TrimPrefix(r.URL.Path, prefix)
	if p == "" && r.Method == http.MethodGet {
		width, height := 640, 480
		var e error
		if q := r.URL.Query().Get("width"); q != "" {
			width, e = strconv.Atoi(q)
			if e != nil {
				return invalid("width")
			}
		}
		if q := r.URL.Query().Get("height"); q != "" {
			height, e = strconv.Atoi(q)
			if e != nil {
				return invalid("height")
			}
		}
		if width != 640 || (height != 480 && height != 180) {
			return invalid("canvas dimensions")
		}
		s := m.stateCopy()
		m.mu.RLock()
		metrics := append([]Metric{}, m.metrics...)
		history := append([]History{}, m.history...)
		m.mu.RUnlock()
		jsonResponse(w, 200, map[string]any{"layouts": sortedLayouts(s), "themes": Themes(width, height), "assets": sortedAssets(s), "metrics": metrics, "history": history, "bindings": s.Bindings, "limits": map[string]int{"max_assets": maxAssets, "max_layouts": maxLayouts, "max_overlays": maxOverlays, "max_frames": maxFrames, "max_asset_bytes": maxAssetBytes, "max_total_bytes": maxTotalBytes, "max_body_bytes": maxBodyBytes, "max_fps": 2}})
		return nil
	}
	if p == "/preview.png" && r.Method == http.MethodPost {
		var l Layout
		if e := body(w, r, &l, 128<<10); e != nil {
			return e
		}
		if e := ValidateLayout(l); e != nil {
			return e
		}
		im, e := m.render(r.Context(), l, time.Now())
		if e != nil {
			return e
		}
		return writePNG(w, im)
	}
	if p == "/assets" && r.Method == http.MethodPost {
		return m.upload(w, r)
	}
	if p == "/apply" && r.Method == http.MethodPost {
		return m.apply(w, r, d)
	}
	parts := strings.Split(strings.TrimPrefix(p, "/"), "/")
	if len(parts) < 2 || len(parts) > 3 || !idPattern.MatchString(parts[1]) {
		return module.ErrNotFound
	}
	id := parts[1]
	if parts[0] == "layouts" {
		if len(parts) == 3 && parts[2] == "preview.png" && r.Method == http.MethodGet {
			im, e := m.Frame(r.Context(), id)
			if e != nil {
				return e
			}
			return writePNG(w, im)
		}
		if len(parts) != 2 {
			return module.ErrNotFound
		}
		if r.Method == http.MethodPut {
			var l Layout
			if e := body(w, r, &l, 128<<10); e != nil {
				return e
			}
			if l.ID != id {
				return invalid("URL and layout id differ")
			}
			if e := ValidateLayout(l); e != nil {
				return e
			}
			m.mutation.Lock()
			defer m.mutation.Unlock()
			s := m.stateCopy()
			if _, ok := s.Layouts[id]; !ok && len(s.Layouts) >= maxLayouts {
				return conflict("layout quota reached")
			}
			if l.Background.AssetID != "" {
				if _, ok := s.Assets[l.Background.AssetID]; !ok {
					return invalid("unknown background asset")
				}
			}
			if previous, exists := s.Layouts[id]; exists && (previous.Width != l.Width || previous.Height != l.Height) {
				for _, binding := range s.Bindings {
					if binding.View == id {
						return conflict("cannot change the canvas size of an assigned layout")
					}
				}
			}
			s.Layouts[id] = l
			if e := m.save(s); e != nil {
				return e
			}
			jsonResponse(w, 200, l)
			return nil
		}
		if r.Method == http.MethodDelete {
			m.mutation.Lock()
			defer m.mutation.Unlock()
			s := m.stateCopy()
			if _, ok := s.Layouts[id]; !ok {
				return module.ErrNotFound
			}
			if id == "default-pump" || id == "default-fan" {
				return conflict("default views cannot be deleted")
			}
			for _, a := range s.Bindings {
				if a.View == id {
					return conflict("layout is assigned to a display")
				}
			}
			delete(s.Layouts, id)
			if e := m.save(s); e != nil {
				return e
			}
			w.WriteHeader(204)
			return nil
		}
	}
	if parts[0] == "assets" {
		if len(parts) == 3 && parts[2] == "preview.png" && r.Method == http.MethodGet {
			m.mu.RLock()
			a, ok := m.assets[id]
			m.mu.RUnlock()
			if !ok {
				return module.ErrNotFound
			}
			im, e := m.assetFrame(a, 0)
			if e != nil {
				return e
			}
			return writePNG(w, im)
		}
		if len(parts) == 2 && r.Method == http.MethodDelete {
			m.mutation.Lock()
			defer m.mutation.Unlock()
			s := m.stateCopy()
			if _, ok := s.Assets[id]; !ok {
				return module.ErrNotFound
			}
			for _, l := range s.Layouts {
				if l.Background.AssetID == id {
					return conflict("asset is referenced by a saved layout")
				}
			}
			delete(s.Assets, id)
			if e := m.save(s); e != nil {
				return e
			}
			// The path is derived exclusively from a validated identifier under assets/.
			if e := os.RemoveAll(filepath.Join(m.dir, "assets", id)); e != nil {
				return fmt.Errorf("asset removed from catalog but file cleanup failed: %w", e)
			}
			w.WriteHeader(204)
			return nil
		}
	}
	return &apiError{http.StatusMethodNotAllowed, "unsupported configurator route or method"}
}
func writePNG(w http.ResponseWriter, im image.Image) error {
	var b bytes.Buffer
	if e := png.Encode(&b, im); e != nil {
		return e
	}
	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(200)
	_, e := w.Write(b.Bytes())
	return e
}
func (m *Module) upload(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Name   string   `json:"name"`
		FPS    int      `json:"fps"`
		Frames []string `json:"frames"`
	}
	if e := body(w, r, &req, maxBodyBytes); e != nil {
		return e
	}
	if strings.TrimSpace(req.Name) == "" || len(req.Name) > 128 || req.FPS < 1 || req.FPS > 2 || len(req.Frames) < 1 || len(req.Frames) > maxFrames || len(req.Frames) > 60*req.FPS {
		return invalid("asset name, fps or frame count")
	}
	m.mutation.Lock()
	defer m.mutation.Unlock()
	s := m.stateCopy()
	if len(s.Assets) >= maxAssets {
		return conflict("asset quota reached")
	}
	var existing int64
	for _, a := range s.Assets {
		existing += a.Bytes
	}
	// Validate and write one frame at a time; decoded full images never accumulate in RAM.
	var random [16]byte
	if _, e := io.ReadFull(rand.Reader, random[:]); e != nil {
		return e
	}
	id := "asset-" + hex.EncodeToString(random[:])
	folder := filepath.Join(m.dir, "assets", id)
	if e := os.Mkdir(folder, 0700); e != nil {
		return e
	}
	success := false
	defer func() {
		if !success {
			os.RemoveAll(folder)
		}
	}()
	a := Asset{ID: id, Name: req.Name, Kind: "image", FrameCount: len(req.Frames), FPS: req.FPS}
	if a.FrameCount > 1 {
		a.Kind = "video"
	}
	for i, encoded := range req.Frames {
		if e := r.Context().Err(); e != nil {
			return e
		}
		if int64(base64.StdEncoding.DecodedLen(len(encoded))) > maxAssetBytes {
			return invalid("asset exceeds 24 MiB")
		}
		data, e := base64.StdEncoding.DecodeString(encoded)
		if e != nil {
			return invalid("frame must be base64 PNG or JPEG")
		}
		req.Frames[i] = ""
		a.Bytes += int64(len(data))
		if a.Bytes > maxAssetBytes {
			return invalid("asset exceeds 24 MiB")
		}
		if existing+a.Bytes > maxTotalBytes {
			return conflict("total media quota reached")
		}
		cfg, format, e := image.DecodeConfig(bytes.NewReader(data))
		if e != nil || (format != "png" && format != "jpeg") || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 640 || cfg.Height > 480 {
			return invalid("frames must be valid PNG/JPEG up to 640x480")
		}
		if i == 0 {
			a.Width, a.Height = cfg.Width, cfg.Height
		} else if a.Width != cfg.Width || a.Height != cfg.Height {
			return invalid("video frames must have equal dimensions")
		}
		if _, _, e := image.Decode(bytes.NewReader(data)); e != nil {
			return invalid("corrupt image frame")
		}
		f, e := os.OpenFile(m.framePath(id, i), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return e
		}
		_, e = f.Write(data)
		if e == nil {
			e = f.Sync()
		}
		closeErr := f.Close()
		if e != nil {
			return e
		}
		if closeErr != nil {
			return closeErr
		}
	}
	s.Assets[id] = a
	if e := m.save(s); e != nil {
		return e
	}
	success = true
	jsonResponse(w, 201, a)
	return nil
}
func (m *Module) apply(w http.ResponseWriter, r *http.Request, d Displays) error {
	if d == nil {
		return module.ErrUnavailable
	}
	var req struct {
		Serial   string `json:"serial"`
		LayoutID string `json:"layout_id"`
		Rotation int    `json:"rotation"`
	}
	if e := body(w, r, &req, 4096); e != nil {
		return e
	}
	if req.Serial == "" || len(req.Serial) > 128 || !idPattern.MatchString(req.LayoutID) || !rotation(req.Rotation) {
		return invalid("display, layout or rotation")
	}
	m.mutation.Lock()
	defer m.mutation.Unlock()
	s := m.stateCopy()
	l, ok := s.Layouts[req.LayoutID]
	if !ok {
		return module.ErrNotFound
	}
	var old module.Assignment
	found := false
	for _, display := range d.Displays() {
		if display.Serial == req.Serial {
			old = display.Assignment
			found = true
			expected := 480
			if display.Kind == "fan" {
				expected = 180
			}
			if l.Height != expected {
				return invalid("layout canvas does not match display")
			}
			break
		}
	}
	if !found {
		return module.ErrNotFound
	}
	if _, ok := s.Bindings[req.Serial]; !ok && len(s.Bindings) >= 128 {
		return conflict("binding quota reached")
	}
	a := module.Assignment{Module: "configurator", View: req.LayoutID, Rotation: req.Rotation}
	s.Bindings[req.Serial] = a
	path, e := m.stage(s)
	if e != nil {
		return e
	}
	defer os.Remove(path)
	if e := d.Assign(req.Serial, a); e != nil {
		return e
	}
	if e := m.commit(path, s); e != nil {
		rollback := d.Assign(req.Serial, old)
		if rollback != nil {
			return fmt.Errorf("save binding: %w; restoring previous assignment also failed: %v", e, rollback)
		}
		return fmt.Errorf("save binding (previous assignment restored): %w", e)
	}
	jsonResponse(w, 200, map[string]any{"serial": req.Serial, "assignment": a})
	return nil
}
