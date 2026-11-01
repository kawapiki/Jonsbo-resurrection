package configurator

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"testing"
	"time"
)

func TestUnsupportedFarewellPreview(t *testing.T) {
	m := fresh(t)
	w := request(t, m, nil, "GET", "/layouts/default-cyclops/farewell.png?ms=0", nil)
	if w.Code != 405 {
		t.Fatalf("expected unsupported operation, got %d: %s", w.Code, w.Body.String())
	}
}

func TestBundledKuromiThemeCanBeImported(t *testing.T) {
	data, err := os.ReadFile("../../themes/kuromi-alive/layout.json")
	if err != nil {
		t.Fatal(err)
	}
	var l Layout
	if err = json.Unmarshal(data, &l); err != nil {
		t.Fatal(err)
	}
	if err = ValidateLayout(l); err != nil {
		t.Fatal(err)
	}
	sprite, err := os.ReadFile("../../themes/kuromi-alive/expressions.png")
	if err != nil {
		t.Fatal(err)
	}
	m := fresh(t)
	w := request(t, m, nil, "POST", "/assets", map[string]any{"name": "Kuromi expressions v1", "fps": 1, "frames": []string{base64.StdEncoding.EncodeToString(sprite)}})
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var a Asset
	if err = json.Unmarshal(w.Body.Bytes(), &a); err != nil {
		t.Fatal(err)
	}
	l.Overlays[0].AssetID = a.ID
	w = request(t, m, nil, "PUT", "/layouts/"+l.ID, l)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, err = m.FarewellFrame(context.Background(), l.ID, 1500*time.Millisecond); err != nil {
		t.Fatal(err)
	}
}

func TestMascotExpressionsAndFarewell(t *testing.T) {
	for _, tc := range []struct {
		ms   int64
		want int
	}{{0, 0}, {2900, 1}, {7700, 1}, {8050, 1}, {17750, 2}, {19500, 0}} {
		got, _, y, size, _ := mascotPose(time.UnixMilli(tc.ms), true, -1)
		if got != tc.want || y < 52 || y > 56 || size != 82 {
			t.Fatal(tc, got, y, size)
		}
	}
	cell, _, y, size, angle := mascotPose(time.UnixMilli(2900), false, -1)
	if cell != 0 || y != 54 || size != 82 || angle != 0 {
		t.Fatal("disabled motion must be still")
	}
	_, x, y, size, angle := mascotPose(time.UnixMilli(2900), true, 1)
	if x != 120 || y != 120 || size != 168 || angle != 0 {
		t.Fatal("farewell not centered", x, y, size, angle)
	}
	o := thermalOverlay()
	v := 60.0
	end := image.NewRGBA(image.Rect(0, 0, 240, 240))
	thermalOrbitAt(end, o, &v, time.Unix(0, 0), 1)
	for y := 90; y < 205; y++ {
		for x := 45; x < 195; x++ {
			if (x-120)*(x-120)+(y-120)*(y-120) < 95*95 && end.RGBAAt(x, y).A != 0 {
				t.Fatal("readings remain during final farewell")
			}
		}
	}
}

func TestMascotAssetPersistsAndCannotBeDeleted(t *testing.T) {
	m := fresh(t)
	var b bytes.Buffer
	png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 600, 200)))
	w := request(t, m, nil, "POST", "/assets", map[string]any{"name": "Expressions", "fps": 1, "frames": []string{base64.StdEncoding.EncodeToString(b.Bytes())}})
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var a Asset
	json.Unmarshal(w.Body.Bytes(), &a)
	o := thermalOverlay()
	o.AssetID = a.ID
	l := Layout{ID: "mascot-test", Name: "Mascot", Width: 240, Height: 240, Background: Background{Color: "#08050E", Opacity: 1}, Overlays: []Overlay{o}}
	w = request(t, m, nil, "PUT", "/layouts/"+l.ID, l)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(t, m, nil, "DELETE", "/assets/"+a.ID, nil)
	if w.Code != 409 {
		t.Fatal("referenced mascot deleted", w.Code)
	}
	reopened, err := New(m.dir, &fakeSource{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = reopened.FarewellFrame(context.Background(), l.ID, 1500*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	for _, ms := range []string{"-1", "1501", "nope"} {
		w = request(t, m, nil, "GET", "/layouts/"+l.ID+"/farewell.png?ms="+ms, nil)
		if w.Code != 400 {
			t.Fatal("unbounded preview", ms, w.Code)
		}
	}
	o.AssetID = "missing"
	l.Overlays = []Overlay{o}
	if w = request(t, m, nil, "PUT", "/layouts/"+l.ID, l); w.Code != 400 {
		t.Fatal("missing asset accepted")
	}
}
