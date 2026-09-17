package api

import (
	"bufio"
	"context"
	"errors"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kawapiki/Jonsbo-resurrection/pkg/module"
)

const testToken = "0123456789abcdef0123456789abcdef"

type fakeBackend struct {
	err        error
	img        image.Image
	unsub      chan struct{}
	block      chan struct{}
	entered    chan struct{}
	eventBlock chan struct{}
}

func (b *fakeBackend) Snapshot() []module.State {
	return []module.State{{Module: module.Descriptor{ID: "demo", Name: "Demo", Version: "1"}, Data: []byte(`{}`)}}
}
func (b *fakeBackend) Subscribe() (<-chan []module.State, func()) {
	return make(chan []module.State), func() {
		if b.unsub != nil {
			close(b.unsub)
		}
	}
}
func (b *fakeBackend) Frame(ctx context.Context, id, view string) (image.Image, error) {
	if b.block != nil {
		b.entered <- struct{}{}
		select {
		case <-b.block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if id != "demo" || view != "main" {
		return nil, module.ErrNotFound
	}
	return b.img, b.err
}
func (b *fakeBackend) Event(_ context.Context, id string, _ module.Event) error {
	if b.eventBlock != nil {
		b.entered <- struct{}{}
		<-b.eventBlock
	}
	if id != "demo" {
		return module.ErrNotFound
	}
	return b.err
}

type fakeDisplays struct{ assignment module.Assignment }

func (d *fakeDisplays) Displays() []module.Display { return nil }
func (d *fakeDisplays) Assign(serial string, a module.Assignment) error {
	if serial != "ABC123" {
		return module.ErrNotFound
	}
	d.assignment = a
	return nil
}
func newTestHandler(t *testing.T, b *fakeBackend, d Displays) http.Handler {
	t.Helper()
	h, err := NewHandler(b, d, testToken)
	if err != nil {
		t.Fatal(err)
	}
	return h
}
func request(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://127.0.0.1:8787"+path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+testToken)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func TestValidateListen(t *testing.T) {
	for _, s := range []string{"127.0.0.1:0", "127.2.3.4:65535", "[::1]:8080"} {
		if err := ValidateListen(s); err != nil {
			t.Errorf("%s: %v", s, err)
		}
	}
	for _, s := range []string{"localhost:80", ":80", "0.0.0.0:80", "192.168.1.1:80", "[::]:80", "127.0.0.1:-1", "127.0.0.1:65536", "127.0.0.1:http", "127.0.0.1", "[::ffff:127.0.0.1]:80"} {
		if ValidateListen(s) == nil {
			t.Errorf("accepted %s", s)
		}
	}
}
func TestSecurity(t *testing.T) {
	if _, err := NewHandler(&fakeBackend{}, nil, "short"); err == nil {
		t.Fatal("short token accepted")
	}
	h := newTestHandler(t, &fakeBackend{}, nil)
	for _, tc := range []struct {
		name, host, auth, origin, path string
		want                           int
	}{
		{"missing", "127.0.0.1:80", "", "", "/v1/state", 401},
		{"query", "127.0.0.1:80", "", "", "/v1/state?token=" + testToken, 401},
		{"bad token", "127.0.0.1:80", "Bearer wrong", "", "/v1/state", 401},
		{"bad host", "evil.test:80", "Bearer " + testToken, "", "/v1/state", 403},
		{"bad origin", "127.0.0.1:80", "Bearer " + testToken, "http://evil.test", "/v1/state", 403},
		{"same origin", "127.0.0.1:80", "Bearer " + testToken, "http://127.0.0.1:80", "/v1/state", 200},
		{"localhost", "localhost:80", "Bearer " + testToken, "", "/v1/state", 200},
		{"ipv6", "[::1]:80", "Bearer " + testToken, "", "/v1/state", 200},
		{"unknown authenticated", "127.0.0.1:80", "Bearer " + testToken, "", "/unknown", 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "http://127.0.0.1"+tc.path, nil)
			r.Host = tc.host
			r.Header.Set("Authorization", tc.auth)
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("got %d want %d", w.Code, tc.want)
			}
			if w.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Fatal("CORS enabled")
			}
		})
	}
}
func TestRoutesAndValidation(t *testing.T) {
	d := &fakeDisplays{}
	h := newTestHandler(t, &fakeBackend{}, d)
	for _, tc := range []struct {
		method, path, body string
		want               int
	}{
		{"GET", "/v1/modules", "", 200}, {"GET", "/v1/state", "", 200}, {"GET", "/v1/modules/demo", "", 200}, {"GET", "/v1/modules/missing", "", 404},
		{"POST", "/v1/modules/demo/events", `{"type":"message","payload":{}}`, 204},
		{"POST", "/v1/modules/demo/events", `{"type":"message","payload":{},"unknown":true}`, 400},
		{"POST", "/v1/modules/demo/events", `{"type":"message","payload":{}} {}`, 400},
		{"POST", "/v1/modules/demo/events", `{"type":"bad type","payload":{}}`, 400},
		{"POST", "/v1/modules/demo/events", `{"type":"message"}`, 400},
		{"POST", "/v1/modules/demo/events", `null`, 400},
		{"POST", "/v1/modules/demo/events", `{"type":"message","payload":"` + strings.Repeat("a", maxBody) + `"}`, 413},
		{"POST", "/v1/modules/demo/events", `{"type":"message","payload":"` + strings.Repeat("a", module.MaxEventBytes) + `"}`, 400},
		{"POST", "/v1/modules/BAD/events", `{}`, 400}, {"POST", "/v1/modules/missing/events", `{"type":"message","payload":{}}`, 404},
		{"GET", "/v1/displays", "", 200},
		{"PUT", "/v1/displays/ABC123/assignment", `{"module":"demo","view":"main","rotation":90}`, 204},
		{"PUT", "/v1/displays/ABC123/assignment", `{"module":"demo","view":"main","rotation":1}`, 400},
		{"PUT", "/v1/displays/missing/assignment", `{"module":"demo","view":"main"}`, 404},
	} {
		w := request(h, tc.method, tc.path, tc.body)
		if w.Code != tc.want {
			t.Errorf("%s %s body %.60s: got %d want %d", tc.method, tc.path, tc.body, w.Code, tc.want)
		}
	}
	if d.assignment.Rotation != 90 {
		t.Fatal("assignment not applied")
	}
	for _, tc := range []struct{ path, allow string }{{"/v1/modules", "GET"}, {"/v1/state", "GET"}, {"/v1/modules/demo", "GET"}, {"/v1/events", "GET"}, {"/v1/modules/demo/views/main.png", "GET"}, {"/v1/modules/demo/events", "POST"}, {"/v1/displays", "GET"}, {"/v1/displays/ABC123/assignment", "PUT"}} {
		w := request(h, "DELETE", tc.path, "")
		if w.Code != 405 || w.Header().Get("Allow") != tc.allow {
			t.Errorf("method %s: %d %s", tc.path, w.Code, w.Header().Get("Allow"))
		}
	}
	nilH := newTestHandler(t, &fakeBackend{}, nil)
	if w := request(nilH, "GET", "/v1/displays", ""); strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatal(w.Body.String())
	}
	if w := request(nilH, "PUT", "/v1/displays/ABC123/assignment", `{}`); w.Code != 405 {
		t.Fatal(w.Code)
	}
}
func TestBackendErrors(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want int
	}{{module.ErrNotFound, 404}, {module.ErrInvalid, 400}, {module.ErrUnavailable, 503}, {module.ErrUnsupported, 405}, {errors.New("SECRET"), 500}} {
		h := newTestHandler(t, &fakeBackend{err: tc.err}, nil)
		w := request(h, "POST", "/v1/modules/demo/events", `{"type":"message","payload":{}}`)
		if w.Code != tc.want || strings.Contains(w.Body.String(), "SECRET") {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}
func TestPNG(t *testing.T) {
	b := &fakeBackend{img: image.NewRGBA(image.Rect(0, 0, 13, 17))}
	h := newTestHandler(t, b, nil)
	w := request(h, "GET", "/v1/modules/demo/views/main.png", "")
	img, err := png.Decode(w.Body)
	if err != nil || img.Bounds().Dx() != 13 || img.Bounds().Dy() != 17 {
		t.Fatal(err)
	}
	if w := request(h, "GET", "/v1/modules/demo/views/missing.png", ""); w.Code != 404 {
		t.Fatal(w.Code)
	}
	b.img = image.NewRGBA(image.Rect(0, 0, 2001, 1000))
	if w := request(h, "GET", "/v1/modules/demo/views/main.png", ""); w.Code != 500 {
		t.Fatal(w.Code)
	}
}
func TestRenderLimit(t *testing.T) {
	b := &fakeBackend{img: image.NewRGBA(image.Rect(0, 0, 1, 1)), block: make(chan struct{}), entered: make(chan struct{}, 2)}
	h := newTestHandler(t, b, nil)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); request(h, "GET", "/v1/modules/demo/views/main.png", "") }()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-b.entered:
		case <-time.After(time.Second):
			t.Fatal("render did not enter")
		}
	}
	w := request(h, "GET", "/v1/modules/demo/views/main.png", "")
	close(b.block)
	wg.Wait()
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
}
func TestSSEDisconnect(t *testing.T) {
	b := &fakeBackend{unsub: make(chan struct{})}
	server := httptest.NewServer(newTestHandler(t, b, nil))
	defer server.Close()
	r, _ := http.NewRequest("GET", server.URL+"/v1/events", nil)
	r.Header.Set("Authorization", "Bearer "+testToken)
	response, err := server.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(response.Body).ReadString('\n')
	if err != nil || line != "event: state\n" {
		t.Fatal(line, err)
	}
	response.Body.Close()
	select {
	case <-b.unsub:
	case <-time.After(2 * time.Second):
		t.Fatal("subscription leaked")
	}
}
func TestSSELimit(t *testing.T) {
	h := newTestHandler(t, &fakeBackend{}, nil).(*handler)
	for i := 0; i < cap(h.streams); i++ {
		h.streams <- struct{}{}
	}
	if w := request(h, "GET", "/v1/events", ""); w.Code != 503 {
		t.Fatal(w.Code)
	}
}

func TestRequestLimitHeldDuringNativeEvent(t *testing.T) {
	b := &fakeBackend{eventBlock: make(chan struct{}), entered: make(chan struct{}, 32)}
	h := newTestHandler(t, b, nil)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			request(h, "POST", "/v1/modules/demo/events", `{"type":"message","payload":{}}`)
		}()
	}
	for i := 0; i < 32; i++ {
		select {
		case <-b.entered:
		case <-time.After(2 * time.Second):
			close(b.eventBlock)
			wg.Wait()
			t.Fatal("event did not enter")
		}
	}
	w := request(h, "POST", "/v1/modules/demo/events", `{"type":"message","payload":{}}`)
	close(b.eventBlock)
	wg.Wait()
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
	if w := request(h, "GET", "/v1/state", ""); w.Code != 200 {
		t.Fatal("request slots leaked", w.Code)
	}
}
