package aiadapters

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	data "github.com/kawapiki/Jonsbo-resurrection/pkg/aisubscriptions"
)

type monitoringRunner struct {
	fixtureRunner
	guard      sync.Mutex
	auth       string
	calls      []string
	usageCalls int
	block      chan struct{}
}

func (r *monitoringRunner) Run(ctx context.Context, _ string, args []string) ([]byte, error) {
	r.guard.Lock()
	r.calls = append(r.calls, args[1])
	auth, block := r.auth, r.block
	r.guard.Unlock()
	if block != nil {
		select {
		case block <- struct{}{}:
		default:
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if args[1] != "status" {
		return nil, errors.New("unexpected login or logout")
	}
	switch auth {
	case "api":
		return []byte(`{"loggedIn":true,"authMethod":"api_key"}`), nil
	case "out":
		return []byte(`{"loggedIn":false,"authMethod":"none"}`), errors.New("native status exits 1 when signed out")
	case "error":
		return nil, errors.New("secret process error")
	default:
		return []byte(`{"loggedIn":true,"authMethod":"claude.ai","accountUuid":"synthetic","subscriptionType":"team"}`), nil
	}
}
func (r *monitoringRunner) ClaudeUsage(context.Context, string) ([]byte, error) {
	r.guard.Lock()
	r.usageCalls++
	r.guard.Unlock()
	return []byte(fixtureClaudeUsage), nil
}

func installFixtureCLI(t *testing.T, directory, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(directory, name+".exe"), []byte("controlled fixture"), 0600); err != nil {
		t.Fatal(err)
	}
}
func monitoringManager(t *testing.T) (*Manager, *monitoringRunner, *[]data.Observation, string) {
	t.Helper()
	directory := t.TempDir()
	t.Setenv("PATH", directory)
	installFixtureCLI(t, directory, "claude")
	m, _, observations := testNativeManager(t)
	t.Setenv("PATH", directory)
	r := &monitoringRunner{fixtureRunner: fixtureRunner{closed: make(chan struct{}), started: make(chan struct{})}}
	m.runner = r
	t.Cleanup(m.cancel)
	return m, r, observations, directory
}
func providerJSON(t *testing.T, m *Manager, provider string) map[string]any {
	t.Helper()
	w := httptest.NewRecorder()
	m.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/v1/ai/status", nil))
	var response struct {
		Providers []map[string]any `json:"providers"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	for _, p := range response.Providers {
		if p["id"] == provider {
			return p
		}
	}
	t.Fatal("missing provider")
	return nil
}
func monitoringAction(m *Manager, provider, action string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	m.Handler().ServeHTTP(w, httptest.NewRequest("POST", "/v1/ai/providers/"+provider+"/"+action, nil))
	return w
}

func TestCheckDetectsNativeLoginWithoutEnablingOrCollecting(t *testing.T) {
	m, r, observations, _ := monitoringManager(t)
	w := monitoringAction(m, data.Claude, "check")
	if w.Code != 200 {
		t.Fatalf("check: %d %s", w.Code, w.Body.String())
	}
	p := providerJSON(t, m, data.Claude)
	if p["login"] != "signed-in" || p["monitoring"] != false || p["connection"] != "disconnected" {
		t.Fatalf("status: %+v", p)
	}
	if r.usageCalls != 0 || len(*observations) != 0 {
		t.Fatal("detection collected or persisted account data")
	}
}

func TestMonitoringReusesNativeLoginAndStopNeverSignsOut(t *testing.T) {
	m, r, observations, _ := monitoringManager(t)
	for _, action := range []string{"monitor", "connect"} {
		w := monitoringAction(m, data.Claude, action)
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", action, w.Code, w.Body.String())
		}
		if providerJSON(t, m, data.Claude)["monitoring"] != true {
			t.Fatal("monitoring did not persist enablement")
		}
	}
	if w := monitoringAction(m, data.Claude, "disconnect"); w.Code != 200 {
		t.Fatalf("stop: %s", w.Body.String())
	}
	p := providerJSON(t, m, data.Claude)
	if p["monitoring"] != false || p["login"] != "signed-in" {
		t.Fatalf("stop changed native login: %+v", p)
	}
	for _, call := range r.calls {
		if call != "status" {
			t.Fatalf("native %s invoked", call)
		}
	}
	if len(*observations) > 0 && (*observations)[len(*observations)-1].Kind != "disconnect" {
		t.Fatal("stop did not detach observations")
	}
	raw, err := os.ReadFile(filepath.Join(m.dir, "connections.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved map[string]bool
	json.Unmarshal(raw, &saved)
	if saved[data.Claude] {
		t.Fatal("stop did not persist")
	}
}

func TestCheckAfterInstallingCLIAndAPIAuthenticationCannotMonitor(t *testing.T) {
	m, r, _, directory := monitoringManager(t)
	os.Remove(filepath.Join(directory, "claude.exe"))
	if w := monitoringAction(m, data.Claude, "check"); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if p := providerJSON(t, m, data.Claude); p["ready"] != false || p["login"] != "unavailable" {
		t.Fatalf("missing CLI: %+v", p)
	}
	installFixtureCLI(t, directory, "claude")
	for auth, want := range map[string]string{"api": "api-key", "out": "signed-out", "error": "error"} {
		r.auth = auth
		if w := monitoringAction(m, data.Claude, "check"); w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		if p := providerJSON(t, m, data.Claude); p["ready"] != true || p["login"] != want || p["monitoring"] != false {
			t.Fatalf("%s: %+v", auth, p)
		}
		for _, action := range []string{"monitor", "connect"} {
			if w := monitoringAction(m, data.Claude, action); w.Code != 503 {
				t.Fatalf("%s %s accepted without existing subscription login", auth, action)
			}
		}
	}
	for _, call := range r.calls {
		if call != "status" {
			t.Fatalf("native %s invoked", call)
		}
	}
}

func TestBackgroundDetectsSignedInWithoutAutoEnabling(t *testing.T) {
	m, r, observations, _ := monitoringManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- m.Run(ctx) }()
	deadline := time.Now().Add(time.Second)
	for providerJSON(t, m, data.Claude)["login"] != "signed-in" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if p := providerJSON(t, m, data.Claude); p["login"] != "signed-in" || p["monitoring"] != false {
		t.Fatalf("background: %+v", p)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("background failed to stop")
	}
	if r.usageCalls != 0 || len(*observations) != 0 {
		t.Fatal("unmonitored background collected account data")
	}
}

func TestCancellationInterruptsUnmonitoredAuthDetection(t *testing.T) {
	m, r, _, _ := monitoringManager(t)
	r.block = make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- m.Run(ctx) }()
	select {
	case <-r.block:
	case <-time.After(time.Second):
		t.Fatal("auth detection did not begin")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancellation did not stop auth detection")
	}
}

func TestCodexDetectionUsesSharedHomeWithoutQuotaOrTokenRefresh(t *testing.T) {
	for _, mode := range []string{"subscription", "api-key", "signed-out"} {
		t.Run(mode, func(t *testing.T) {
			m, r, observations := testNativeManager(t)
			defer m.cancel()
			defer func() {
				if m.rpc != nil {
					m.rpc.Close()
				}
			}()
			r.accountType = mode
			home := filepath.Join(t.TempDir(), "shared-native-home")
			t.Setenv("CODEX_HOME", home)
			if w := monitoringAction(m, data.OpenAI, "check"); w.Code != 200 {
				t.Fatal(w.Body.String())
			}
			want := "signed-in"
			if mode == "api-key" {
				want = "api-key"
			}
			if mode == "signed-out" {
				want = "signed-out"
			}
			if p := providerJSON(t, m, data.OpenAI); p["login"] != want || p["monitoring"] != false {
				t.Fatalf("%+v", p)
			}
			if len(*observations) != 0 {
				t.Fatal("detection published account data")
			}
			if mode != "subscription" {
				for _, action := range []string{"monitor", "connect"} {
					if w := monitoringAction(m, data.OpenAI, action); w.Code != 503 {
						t.Fatalf("%s accepted %s", action, mode)
					}
				}
			}
			r.mu.Lock()
			defer r.mu.Unlock()
			if r.home != home {
				t.Fatalf("native home: %s", r.home)
			}
			for _, flag := range r.refreshFlags {
				if flag {
					t.Fatal("detection refreshed native tokens")
				}
			}
			for _, method := range r.methods {
				if method != "initialize" && method != "initialized" && method != "account/read" {
					t.Fatalf("unexpected native request %s", method)
				}
			}
		})
	}
}

func TestNativeCodexHomeDefaultsToNativeUserDirectory(t *testing.T) {
	t.Setenv("CODEX_HOME", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	got, err := nativeCodexHome()
	if err != nil || got != filepath.Join(home, ".codex") {
		t.Fatalf("home %q %v", got, err)
	}
}

func TestCheckReopensCodexUsingCurrentNativeHome(t *testing.T) {
	m, r, _ := testNativeManager(t)
	defer m.cancel()
	defer func() {
		if m.rpc != nil {
			m.rpc.Close()
		}
	}()
	for _, home := range []string{filepath.Join(t.TempDir(), "first"), filepath.Join(t.TempDir(), "changed")} {
		t.Setenv("CODEX_HOME", home)
		if w := monitoringAction(m, data.OpenAI, "check"); w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		r.mu.Lock()
		actual := r.home
		r.mu.Unlock()
		if actual != home {
			t.Fatalf("check reused previous native home: %s", actual)
		}
	}
}

func TestExistingSavedMonitoringUpdatesLoginDuringRefresh(t *testing.T) {
	m, r, _ := testNativeManager(t)
	defer m.cancel()
	defer func() {
		if m.rpc != nil {
			m.rpc.Close()
		}
	}()
	m.enabled[data.OpenAI] = true
	if err := m.refresh(data.OpenAI); err != nil {
		t.Fatal(err)
	}
	if p := providerJSON(t, m, data.OpenAI); p["login"] != "signed-in" || p["monitoring"] != true {
		t.Fatalf("saved monitoring: %+v", p)
	}
	r.mu.Lock()
	r.accountType = "api-key"
	r.mu.Unlock()
	if err := m.refresh(data.OpenAI); err == nil {
		t.Fatal("API auth refresh accepted")
	}
	if p := providerJSON(t, m, data.OpenAI); p["login"] != "api-key" || p["monitoring"] != true || p["connection"] != "reauthentication" {
		t.Fatalf("lost login issue: %+v", p)
	}
}

func TestEnabledCodexPollReopensStartupCachedAuthentication(t *testing.T) {
	m, r, observations := testNativeManager(t)
	defer m.cancel()
	defer func() {
		if m.rpc != nil {
			m.rpc.Close()
		}
	}()
	m.enabled[data.OpenAI] = true
	r.snapshotAuth = true
	r.accountEmail = "first@example.test"
	r.quotaAccountID = "first-account"
	firstHome := filepath.Join(t.TempDir(), "first-native-home")
	t.Setenv("CODEX_HOME", firstHome)
	if err := m.refresh(data.OpenAI); err != nil {
		t.Fatal(err)
	}
	firstKey := (*observations)[0].AccountKey
	beforeLogout := len(*observations)
	r.mu.Lock()
	r.accountType = "signed-out"
	r.mu.Unlock()
	if err := m.refresh(data.OpenAI); err == nil {
		t.Fatal("enabled poll missed external logout cached by native worker")
	}
	if p := providerJSON(t, m, data.OpenAI); p["login"] != "signed-out" || p["connection"] != "reauthentication" {
		t.Fatalf("logout status: %+v", p)
	}
	if len(*observations) != beforeLogout+1 || (*observations)[beforeLogout].Connection != "reauthentication" {
		t.Fatal("logout published previous account usage")
	}
	r.mu.Lock()
	r.accountType = "subscription"
	r.accountEmail = "second@example.test"
	r.quotaAccountID = "second-account"
	r.mu.Unlock()
	secondHome := filepath.Join(t.TempDir(), "second-native-home")
	t.Setenv("CODEX_HOME", secondHome)
	beforeRelogin := len(*observations)
	if err := m.refresh(data.OpenAI); err != nil {
		t.Fatal(err)
	}
	if p := providerJSON(t, m, data.OpenAI); p["login"] != "signed-in" || p["monitoring"] != true {
		t.Fatalf("relogin status: %+v", p)
	}
	if (*observations)[beforeRelogin].AccountKey == firstKey {
		t.Fatal("external relogin reused previous account identity")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.home != secondHome || r.workerStarts != 3 {
		t.Fatalf("enabled poll reused cached auth/home: home=%s workers=%d", r.home, r.workerStarts)
	}
	for _, method := range r.methods {
		if method != "initialize" && method != "initialized" && method != "account/read" && method != "account/rateLimits/read" && method != "account/usage/read" {
			t.Fatalf("unexpected native method %s", method)
		}
	}
}
