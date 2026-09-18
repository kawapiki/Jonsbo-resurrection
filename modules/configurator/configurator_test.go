package configurator

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kawapiki/Jonsbo-resurrection/pkg/module"
)

type fakeSource struct{ states []module.State }

func (s *fakeSource) Snapshot() []module.State { return s.states }

type fakeDisplays struct {
	assignment module.Assignment
	err        error
}

func TestImageDimensionsFrameCountAndQuota(t *testing.T) {
	m := fresh(t)
	d := &fakeDisplays{}
	var b bytes.Buffer
	png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 641, 480)))
	tooWide := base64.StdEncoding.EncodeToString(b.Bytes())
	for _, frames := range [][]string{{tooWide}, make([]string, 121)} {
		w := request(t, m, d, "POST", "/assets", map[string]any{"name": "Invalid", "fps": 2, "frames": frames})
		if w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	for i := 0; i < maxAssets; i++ {
		id := fmt.Sprintf("asset-%d", i)
		m.assets[id] = Asset{ID: id, Bytes: 1}
	}
	if w := request(t, m, d, "POST", "/assets", map[string]any{"name": "Full", "fps": 1, "frames": []string{frame64(t, color.RGBA{255, 0, 0, 255})}}); w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestPreviewBackgroundAndTextPixels(t *testing.T) {
	m := fresh(t)
	d := &fakeDisplays{}
	w := request(t, m, d, "POST", "/assets", map[string]any{"name": "Image", "fps": 1, "frames": []string{frame64(t, color.RGBA{255, 0, 0, 255})}})
	var a Asset
	if e := json.Unmarshal(w.Body.Bytes(), &a); e != nil {
		t.Fatal(e)
	}
	l := Themes(640, 480)[0].Layout
	l.Background.AssetID = a.ID
	l.Overlays = nil
	im, e := m.render(context.Background(), l, time.Unix(0, 0))
	if e != nil {
		t.Fatal(e)
	}
	r, g, b, _ := im.At(0, 0).RGBA()
	if r < 60000 || g != 0 || b != 0 {
		t.Fatalf("background pixel %d,%d,%d", r, g, b)
	}
	canvas := image.NewRGBA(image.Rect(0, 0, 100, 30))
	text(canvas, 0, 0, "CPU 42%", 14, color.RGBA{255, 255, 255, 255})
	count := 0
	for _, v := range canvas.Pix {
		if v != 0 {
			count++
		}
	}
	if count < 300 {
		t.Fatal("text not visibly rasterized")
	}
	if c := rgba("#80C0FF", 1); c != (color.RGBA{128, 192, 255, 255}) {
		t.Fatal("incorrect palette", c)
	}
}

func TestRenderConcurrentWithEdits(t *testing.T) {
	m := fresh(t)
	d := &fakeDisplays{}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 8; j++ {
				if _, e := m.Frame(context.Background(), "default-pump"); e != nil {
					t.Error(e)
				}
			}
		}()
	}
	for i := 0; i < 8; i++ {
		l := Themes(640, 480)[i%5].Layout
		l.ID = "default-pump"
		if w := request(t, m, d, "PUT", "/layouts/default-pump", l); w.Code != 200 {
			t.Fatal(w.Body.String())
		}
	}
	wg.Wait()
}

func TestStrictJSONAndSavedPathInjection(t *testing.T) {
	m := fresh(t)
	w := request(t, m, &fakeDisplays{}, "PUT", "/layouts/test", map[string]any{"id": "test", "unexpected": 1})
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
	s := m.stateCopy()
	s.Assets["../outside"] = Asset{ID: "../outside", Name: "Unsafe", FrameCount: 1, FPS: 1, Width: 1, Height: 1, Kind: "image", Bytes: 1}
	b, _ := json.Marshal(s)
	if e := os.WriteFile(filepath.Join(m.dir, "state.json"), b, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := New(m.dir, &fakeSource{}); e == nil {
		t.Fatal("saved path injection allowed")
	}
}

func (d *fakeDisplays) Displays() []module.Display {
	return []module.Display{{Serial: "test", Kind: "pump", Assignment: d.assignment}}
}
func (d *fakeDisplays) Assign(_ string, a module.Assignment) error {
	if d.err != nil {
		return d.err
	}
	d.assignment = a
	return nil
}
func request(t *testing.T, m *Module, d *fakeDisplays, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(method, "/v1/configurator"+path, bytes.NewReader(b))
	w := httptest.NewRecorder()
	m.Handler(d).ServeHTTP(w, r)
	return w
}
func fresh(t *testing.T) *Module {
	t.Helper()
	m, e := New(t.TempDir(), &fakeSource{})
	if e != nil {
		t.Fatal(e)
	}
	return m
}
func TestLayoutValidationAndPersistence(t *testing.T) {
	dir := t.TempDir()
	m, e := New(dir, &fakeSource{})
	if e != nil {
		t.Fatal(e)
	}
	d := &fakeDisplays{}
	l := Themes(640, 480)[0].Layout
	l.ID = "custom"
	l.Name = "First"
	if w := request(t, m, d, "PUT", "/layouts/custom", l); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	l.Name = "Replaced"
	if w := request(t, m, d, "PUT", "/layouts/custom", l); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	n, e := New(dir, &fakeSource{})
	if e != nil {
		t.Fatal(e)
	}
	if n.layouts["custom"].Name != "Replaced" {
		t.Fatal("replacement not durable")
	}
	l.Overlays[0].X = -1
	if w := request(t, m, d, "PUT", "/layouts/custom", l); w.Code != 400 {
		t.Fatal(w.Code)
	}
	l = Themes(640, 480)[0].Layout
	l.ID = "../escape"
	if e := ValidateLayout(l); e == nil {
		t.Fatal("path accepted")
	}
}
func frame64(t *testing.T, c color.RGBA) string {
	t.Helper()
	im := image.NewRGBA(image.Rect(0, 0, 2, 2))
	im.Set(0, 0, c)
	var b bytes.Buffer
	if e := png.Encode(&b, im); e != nil {
		t.Fatal(e)
	}
	return base64.StdEncoding.EncodeToString(b.Bytes())
}
func TestAssetsDecodeBoundsLoopAndRestart(t *testing.T) {
	m := fresh(t)
	d := &fakeDisplays{}
	frames := []string{frame64(t, color.RGBA{255, 0, 0, 255}), frame64(t, color.RGBA{0, 255, 0, 255})}
	w := request(t, m, d, "POST", "/assets", map[string]any{"name": "Loop", "fps": 2, "frames": frames})
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var a Asset
	json.Unmarshal(w.Body.Bytes(), &a)
	if a.FrameCount != 2 || frameIndex(a, time.Unix(0, 0)) != 0 || frameIndex(a, time.Unix(0, 500000000)) != 1 || frameIndex(a, time.Unix(1, 0)) != 0 {
		t.Fatal("loop indices")
	}
	n, e := New(m.dir, &fakeSource{})
	if e != nil {
		t.Fatal(e)
	}
	if len(n.assets) != 1 {
		t.Fatal("asset lost")
	}
	for _, bad := range []any{map[string]any{"name": "Bad", "fps": 2, "frames": []string{"notbase64"}}, map[string]any{"name": "Bad", "fps": 2, "frames": []string{base64.StdEncoding.EncodeToString([]byte("brokenpng"))}}, map[string]any{"name": "Bad", "fps": 3, "frames": frames}} {
		if w := request(t, m, d, "POST", "/assets", bad); w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if w := request(t, m, d, "GET", "/assets/..%2f..%2ftoken/preview.png", nil); w.Code == 200 {
		t.Fatal("unsafe path accepted")
	}
	l := Themes(640, 480)[0].Layout
	l.ID = "media"
	l.Background.AssetID = a.ID
	request(t, m, d, "PUT", "/layouts/media", l)
	if w := request(t, m, d, "DELETE", "/assets/"+a.ID, nil); w.Code != 409 {
		t.Fatal("referenced asset deleted", w.Code)
	}
}
func TestThemeRotationMatchesNativePanel(t *testing.T) {
	for _, height := range []int{180, 480} {
		want := 0
		if height == 180 {
			want = 90
		}
		for _, theme := range Themes(640, height) {
			if theme.Layout.Rotation != want {
				t.Fatalf("%s height %d rotation = %d, want %d", theme.ID, height, theme.Layout.Rotation, want)
			}
		}
	}
}

func TestApplyFailureAndBindingRestart(t *testing.T) {
	m := fresh(t)
	d := &fakeDisplays{err: errors.New("offline")}
	body := map[string]any{"serial": "test", "layout_id": "default-pump", "rotation": 0}
	if w := request(t, m, d, "POST", "/apply", body); w.Code < 400 {
		t.Fatal(w.Body.String())
	}
	if len(m.Bindings()) != 0 {
		t.Fatal("failed apply committed")
	}
	d.err = nil
	if w := request(t, m, d, "POST", "/apply", body); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	n, e := New(m.dir, &fakeSource{})
	if e != nil {
		t.Fatal(e)
	}
	if n.Bindings()["test"].View != "default-pump" {
		t.Fatal("binding lost")
	}
}
func TestHistoryMissingAndBounded(t *testing.T) {
	src := &fakeSource{}
	m, e := New(t.TempDir(), src)
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now()
	src.states = []module.State{{Module: module.Descriptor{ID: "hardware"}, Status: "running", UpdatedAt: now, Data: json.RawMessage(`{"cpu":{"usage_percent":25},"memory":{"total_bytes":1073741824,"used_bytes":536870912,"usage_percent":50},"gpus":[]}`)}}
	m.sample(now)
	if m.history[0].Values["cpu.usage"] == nil || *m.history[0].Values["cpu.usage"] != 25 {
		t.Fatal("missing cpu")
	}
	m.sample(now.Add(10 * time.Second))
	if m.history[1].Values["cpu.usage"] != nil {
		t.Fatal("stale data became live")
	}
	for i := 0; i < 1300; i++ {
		m.sample(now.Add(time.Duration(i) * time.Second))
	}
	if len(m.history) > 600 {
		t.Fatal("unbounded history")
	}
}
func TestThemesRenderDistinctAndReadable(t *testing.T) {
	m := fresh(t)
	seen := map[string]bool{}
	for _, h := range []int{180, 480} {
		for _, th := range Themes(640, h) {
			if e := ValidateLayout(th.Layout); e != nil {
				t.Fatal(th.ID, e)
			}
			im, e := m.render(context.Background(), th.Layout, time.Now())
			if e != nil {
				t.Fatal(e)
			}
			var b bytes.Buffer
			png.Encode(&b, im)
			if seen[b.String()] {
				t.Fatal("identical themes")
			}
			seen[b.String()] = true
			if im.Bounds().Dy() != h {
				t.Fatal("wrong height")
			}
		}
	}
}
func TestFailedSavePreservesMemory(t *testing.T) {
	m := fresh(t)
	before := m.layouts["default-pump"].Name
	if e := os.Mkdir(filepath.Join(m.dir, "state.json"), 0700); e != nil {
		t.Fatal(e)
	}
	l := m.layouts["default-pump"]
	l.Name = "Must not commit"
	w := request(t, m, &fakeDisplays{}, "PUT", "/layouts/default-pump", l)
	if w.Code < 400 {
		t.Fatal("write unexpectedly succeeded")
	}
	if m.layouts["default-pump"].Name != before {
		t.Fatal("failed write changed memory")
	}
}

func TestApplyCommitFailureRollsBackOutput(t *testing.T) {
	m := fresh(t)
	old := module.Assignment{Module: "hardware", View: "summary", Rotation: 180}
	d := &fakeDisplays{assignment: old}
	if err := os.Mkdir(filepath.Join(m.dir, "state.json"), 0700); err != nil {
		t.Fatal(err)
	}
	w := request(t, m, d, "POST", "/apply", map[string]any{"serial": "test", "layout_id": "default-pump", "rotation": 0})
	if w.Code != 500 {
		t.Fatal(w.Code, w.Body.String())
	}
	if d.assignment != old || len(m.Bindings()) != 0 {
		t.Fatal("commit failure left new output or binding")
	}
}

func TestInterruptedUploadCleanup(t *testing.T) {
	dir := t.TempDir()
	orphan := filepath.Join(dir, "assets", "asset-01234567890123456789012345678901")
	if e := os.MkdirAll(orphan, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(orphan, "000.frame"), []byte("unfinished"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := New(dir, &fakeSource{}); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(orphan); !os.IsNotExist(e) {
		t.Fatal("orphan not reclaimed", e)
	}
}

func TestHTTPBodyAndAggregateMediaLimits(t *testing.T) {
	m := fresh(t)
	d := &fakeDisplays{}
	r := httptest.NewRequest("POST", "/v1/configurator/assets", strings.NewReader(strings.Repeat(" ", maxBodyBytes+1)))
	w := httptest.NewRecorder()
	m.Handler(d).ServeHTTP(w, r)
	if w.Code != 413 {
		t.Fatal("oversized body", w.Code, w.Body.String())
	}
	for i := 0; i < 6; i++ {
		id := fmt.Sprintf("asset-%d", i)
		m.assets[id] = Asset{ID: id, Bytes: maxAssetBytes}
	}
	w = request(t, m, d, "POST", "/assets", map[string]any{"name": "Full", "fps": 1, "frames": []string{frame64(t, color.RGBA{255, 0, 0, 255})}})
	if w.Code != 409 {
		t.Fatal("aggregate quota", w.Code, w.Body.String())
	}
	entries, e := os.ReadDir(filepath.Join(m.dir, "assets"))
	if e != nil {
		t.Fatal(e)
	}
	if len(entries) != 0 {
		t.Fatal("failed upload left staged media")
	}
}
