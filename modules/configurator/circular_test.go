package configurator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/kawapiki/Jonsbo-resurrection/pkg/module"
	"image/color"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCircularThemesRenderAndPersist(t *testing.T) {
	m := fresh(t)
	themes := Themes(240, 240)
	if len(themes) < 4 {
		t.Fatal("need distinct round presets")
	}
	for _, theme := range themes {
		l := theme.Layout
		if l.Width != 240 || l.Height != 240 {
			t.Fatal("not a native circular layout", l.Width, l.Height)
		}
		if err := ValidateLayout(l); err != nil {
			t.Fatal(theme.ID, err)
		}
		im, err := m.render(context.Background(), l, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if im.At(0, 0) != (color.RGBA{0, 0, 0, 255}) {
			t.Fatal("outside circle must be black")
		}
		w := request(t, m, &fakeDisplays{}, "PUT", "/layouts/"+l.ID, l)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	reopened, err := New(m.dir, &fakeSource{})
	if err != nil {
		t.Fatal(err)
	}
	if err = reopened.ValidateView(themes[0].ID); err != nil {
		t.Fatal(err)
	}
	if err = reopened.ValidateView("default-cyclops"); err != nil {
		t.Fatal("missing default", err)
	}
}

type roundDisplays struct{ assignment module.Assignment }

func (d *roundDisplays) Displays() []module.Display {
	return []module.Display{{Serial: "RJK-TEST", Kind: "cyclops", Assignment: d.assignment}}
}
func (d *roundDisplays) Assign(_ string, a module.Assignment) error { d.assignment = a; return nil }

func TestCircularAssignmentRejectsRectangularLayout(t *testing.T) {
	m := fresh(t)
	d := &roundDisplays{}
	for _, tc := range []struct {
		id   string
		code int
	}{{"default-pump", 400}, {"default-fan", 400}, {"default-cyclops", 200}} {
		payload, _ := json.Marshal(map[string]any{"serial": "RJK-TEST", "layout_id": tc.id, "rotation": 90})
		w := httptest.NewRecorder()
		m.Handler(d).ServeHTTP(w, httptest.NewRequest("POST", "/v1/configurator/apply", bytes.NewReader(payload)))
		if w.Code != tc.code {
			t.Fatal(tc.id, w.Code, w.Body.String())
		}
	}
	if d.assignment.View != "default-cyclops" || d.assignment.Rotation != 90 {
		t.Fatal(d.assignment)
	}
}

func TestCircularCatalog(t *testing.T) {
	w := request(t, fresh(t), &fakeDisplays{}, "GET", "?width=240&height=240", nil)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestCircularDefaultAddedToFullLegacyLibrary(t *testing.T) {
	m := fresh(t)
	s := m.stateCopy()
	delete(s.Layouts, "default-cyclops")
	for len(s.Layouts) < 32 {
		l := Themes(640, 480)[0].Layout
		l.ID = fmt.Sprintf("legacy-%d", len(s.Layouts))
		s.Layouts[l.ID] = l
	}
	if err := m.save(s); err != nil {
		t.Fatal(err)
	}
	n, err := New(m.dir, &fakeSource{})
	if err != nil {
		t.Fatal(err)
	}
	if err = n.ValidateView("default-cyclops"); err != nil {
		t.Fatal("full existing library lost circular default", err)
	}
}
