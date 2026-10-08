package aiadapters

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"github.com/kawapiki/Jonsbo-resurrection/modules/aisubscriptions"
	data "github.com/kawapiki/Jonsbo-resurrection/pkg/aisubscriptions"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type fixtureRunner struct {
	mu             sync.Mutex
	methods        []string
	closed         chan struct{}
	started        chan struct{}
	failQuotas     bool
	accountEmail   string
	quotaAccountID string
	accountType    string
	home           string
	refreshFlags   []bool
	startOnce      sync.Once
	closeOnce      sync.Once
	snapshotAuth   bool
	workerStarts   int
	accountDelay   time.Duration
}

func TestSlowCodexAuthenticationDoesNotDiscardFreshQuota(t *testing.T) {
	m, runner, _ := testNativeManager(t)
	defer m.cancel()
	defer func() {
		if m.rpc != nil {
			m.rpc.Close()
		}
	}()
	aggregate, err := aisubscriptions.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m.sink = aggregate.Observe
	m.enabled[data.OpenAI] = true
	runner.accountDelay = 1200 * time.Millisecond
	if err := m.refresh(data.OpenAI); err != nil {
		t.Fatal(err)
	}
	p := aggregate.Snapshot().Providers[0]
	if len(p.Quotas) != 1 || p.Quotas[0].UsedPercent == nil || *p.Quotas[0].UsedPercent != 42 {
		t.Fatal("fresh native quota was discarded after a slow authentication check")
	}
	if p.Quotas[0].ObservedAt.Before(p.UpdatedAt) || p.Stale {
		t.Fatal("completed quota reading was timestamped before authentication")
	}
}

func TestNativeRefreshReportsRejectedUsageIntake(t *testing.T) {
	m, _, _ := testNativeManager(t)
	defer m.cancel()
	defer func() {
		if m.rpc != nil {
			m.rpc.Close()
		}
	}()
	m.enabled[data.OpenAI] = true
	m.sink = func(_ context.Context, o data.Observation) error {
		if o.Kind == "usage" {
			return errors.New("synthetic intake rejection")
		}
		return nil
	}
	if err := m.refresh(data.OpenAI); err == nil {
		t.Fatal("refresh swallowed rejected normalized usage")
	}
}

func (f *fixtureRunner) Run(ctx context.Context, _ string, args []string) ([]byte, error) {
	if len(args) > 1 && args[1] == "login" {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return []byte(`{"loggedIn":true,"authMethod":"claude.ai","accountUuid":"claude-id","subscriptionType":"max"}`), nil
}
func (f *fixtureRunner) StartCodex(_ context.Context, _ string, home string, notify func(string, json.RawMessage)) (*rpcClient, error) {
	f.mu.Lock()
	f.home = home
	f.workerStarts++
	snapshotAuth, initialType, initialEmail, initialQuotaID := f.snapshotAuth, f.accountType, f.accountEmail, f.quotaAccountID
	f.mu.Unlock()
	input, serverOutput := io.Pipe()
	serverInput, output := io.Pipe()
	r := newRPC(input, output, func() {
		serverInput.Close()
		serverOutput.Close()
		if f.closed != nil {
			f.closeOnce.Do(func() { close(f.closed) })
		}
	}, notify)
	go func() {
		if f.started != nil {
			f.startOnce.Do(func() { close(f.started) })
		}
		scanner := bufio.NewScanner(serverInput)
		encoder := json.NewEncoder(serverOutput)
		for scanner.Scan() {
			var req struct {
				ID     uint64         `json:"id"`
				Method string         `json:"method"`
				Params map[string]any `json:"params"`
			}
			json.Unmarshal(scanner.Bytes(), &req)
			f.mu.Lock()
			f.methods = append(f.methods, req.Method)
			failQuotas, email, quotaID := f.failQuotas, f.accountEmail, f.quotaAccountID
			accountDelay := f.accountDelay
			accountType := f.accountType
			if snapshotAuth {
				accountType, email, quotaID = initialType, initialEmail, initialQuotaID
			}
			if req.Method == "account/read" {
				flag, _ := req.Params["refreshToken"].(bool)
				f.refreshFlags = append(f.refreshFlags, flag)
			}
			f.mu.Unlock()
			if req.ID == 0 {
				continue
			}
			var result any = map[string]any{}
			switch req.Method {
			case "account/read":
				time.Sleep(accountDelay)
				if accountType == "signed-out" {
					result = map[string]any{"account": nil}
					break
				}
				if accountType == "api-key" {
					result = map[string]any{"account": map[string]string{"type": "apiKey"}}
					break
				}
				if email == "" {
					email = "verified@example.test"
				}
				result = map[string]any{"account": map[string]string{"type": "chatgpt", "email": email, "planType": "plus"}}
			case "account/rateLimits/read":
				if failQuotas {
					encoder.Encode(map[string]any{"id": req.ID, "error": map[string]any{"code": -32601}})
					continue
				}
				if quotaID == "" {
					quotaID = "account-id"
				}
				result = map[string]any{"accountId": quotaID, "rateLimits": map[string]any{"primary": map[string]any{"usedPercent": 42, "windowDurationMins": 10080}}}
			case "account/usage/read":
				result = map[string]any{"dailyUsageBuckets": []any{map[string]any{"startDate": "2026-10-07", "tokens": 1234}}}
			}
			encoder.Encode(map[string]any{"id": req.ID, "result": result})
		}
	}()
	return r, nil
}

func TestOptionalQuotaFailureKeepsVerifiedAccountAlias(t *testing.T) {
	m, f, observations := testNativeManager(t)
	defer m.cancel()
	m.enabled[data.OpenAI] = true
	defer func() {
		if m.rpc != nil {
			m.rpc.Close()
		}
	}()
	if err := m.refresh(data.OpenAI); err != nil {
		t.Fatal(err)
	}
	firstKey := (*observations)[0].AccountKey
	f.mu.Lock()
	f.failQuotas = true
	f.mu.Unlock()
	if err := m.refresh(data.OpenAI); err != nil {
		t.Fatal(err)
	}
	if (*observations)[3].AccountKey != firstKey {
		t.Fatalf("optional quota failure changed identity from %s to %s", firstKey, (*observations)[3].AccountKey)
	}
	f.mu.Lock()
	f.accountEmail = "different@example.test"
	f.mu.Unlock()
	if err := m.refresh(data.OpenAI); err != nil {
		t.Fatal(err)
	}
	if (*observations)[5].AccountKey == firstKey {
		t.Fatal("different verified account reused previous alias")
	}
}

func TestVerifiedAccountAliasSurvivesRestartAndClearsOnMonitoringStop(t *testing.T) {
	m, _, _ := testNativeManager(t)
	defer m.cancel()
	m.enabled[data.OpenAI] = true
	if err := m.refresh(data.OpenAI); err != nil {
		t.Fatal(err)
	}
	m.rpc.Close()
	m.rpc = nil
	var observations []data.Observation
	m2, err := New(m.dir, func(_ context.Context, o data.Observation) error { observations = append(observations, o); return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer m2.cancel()
	f2 := &fixtureRunner{closed: make(chan struct{}), started: make(chan struct{}), failQuotas: true}
	m2.runner = f2
	m2.providers[data.OpenAI] = ProviderStatus{ID: data.OpenAI, Ready: true}
	m2.enabled[data.OpenAI] = true
	if err = m2.refresh(data.OpenAI); err != nil {
		t.Fatal(err)
	}
	if observations[0].AccountKey != opaque(data.OpenAI, "account-id") {
		m2.rpc.Close()
		t.Fatal("restart lost verified account alias")
	}
	if err = m2.disconnect(data.OpenAI); err != nil {
		t.Fatal(err)
	}
	m3, err := New(m.dir, func(context.Context, data.Observation) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer m3.cancel()
	if len(m3.accountAliases) != 0 {
		t.Fatal("stopping monitoring retained cached account identity")
	}
}
func testNativeManager(t *testing.T) (*Manager, *fixtureRunner, *[]data.Observation) {
	t.Helper()
	var observations []data.Observation
	m, err := New(t.TempDir(), func(_ context.Context, o data.Observation) error { observations = append(observations, o); return nil })
	if err != nil {
		t.Fatal(err)
	}
	f := &fixtureRunner{closed: make(chan struct{}), started: make(chan struct{})}
	m.runner = f
	directory := t.TempDir()
	t.Setenv("PATH", directory)
	for _, p := range []string{data.OpenAI, data.Claude} {
		name := "codex"
		if p == data.Claude {
			name = "claude"
		}
		if err := os.WriteFile(filepath.Join(directory, name+".exe"), []byte("controlled fixture"), 0600); err != nil {
			t.Fatal(err)
		}
		m.providers[p] = ProviderStatus{ID: p, Ready: true, Connection: "disconnected"}
		m.paths[p] = "fixture-native"
	}
	return m, f, &observations
}
func TestExistingNativeLoginAndAccountPollNeverStartModelTurn(t *testing.T) {
	m, f, observations := testNativeManager(t)
	defer m.cancel()
	result, err := m.connect(data.OpenAI)
	if err != nil || result["auth_url"] != "" || result["accessToken"] != "" {
		t.Fatalf("%+v %v", result, err)
	}
	if err = m.refresh(data.OpenAI); err != nil {
		t.Fatal(err)
	}
	if len(*observations) != 3 || (*observations)[0].AccountKey != opaque(data.OpenAI, "account-id") || (*observations)[1].Quotas[0].WindowMinutes == nil || (*observations)[2].DailyUsage[0].Tokens != 1234 {
		t.Fatalf("%+v", *observations)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, method := range f.methods {
		if method != "initialize" && method != "initialized" && method != "account/read" && method != "account/rateLimits/read" && method != "account/usage/read" {
			t.Fatalf("unexpected request %s", method)
		}
	}
	m.rpc.Close()
}
func TestRunCancellationStopsNativeAndRejectsFurtherIntake(t *testing.T) {
	m, f, _ := testNativeManager(t)
	m.enabled[data.OpenAI] = true
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- m.Run(ctx) }()
	select {
	case <-f.started:
	case <-time.After(time.Second):
		t.Fatal("native worker did not start")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("manager cancellation blocked")
	}
	select {
	case <-f.closed:
	default:
		t.Fatal("native process not closed")
	}
	if m.emit(context.Background(), data.Observation{}) == nil {
		t.Fatal("closed manager accepted event")
	}
}
func TestParserRejectsTrailingGarbageAndAbsentUsageStaysNull(t *testing.T) {
	if _, err := ProjectNative(data.Claude, "hook", []byte(`{"session_id":"s","hook_event_name":"Stop"} trailing`), time.Now()); err == nil {
		t.Fatal("trailing payload accepted")
	}
	obs, err := ProjectNative(data.Claude, "statusline", []byte(`{"session_id":"s","context_window":{"current_usage":{}}}`), time.Now())
	if err != nil || obs[0].Context.Used != nil {
		t.Fatalf("invented context %+v %v", obs, err)
	}
}

type blockedRefreshRunner struct {
	fixtureRunner
	entered chan struct{}
}

func (r *blockedRefreshRunner) Run(ctx context.Context, _ string, _ []string) ([]byte, error) {
	close(r.entered)
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestParentCancellationInterruptsBlockedRefresh(t *testing.T) {
	m, _, _ := testNativeManager(t)
	defer m.cancel()
	runner := &blockedRefreshRunner{entered: make(chan struct{})}
	m.runner = runner
	m.enabled[data.Claude] = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- m.Run(ctx) }()
	select {
	case <-runner.entered:
	case <-time.After(time.Second):
		t.Fatal("refresh did not enter native command")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		m.cancel()
		<-done
		t.Fatal("parent cancellation did not interrupt blocked native refresh")
	}
}
