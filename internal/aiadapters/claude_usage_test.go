package aiadapters

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	data "github.com/kawapiki/Jonsbo-resurrection/pkg/aisubscriptions"
)

const fixtureClaudeUsage = `{"subscription_type":"team","rate_limits_available":true,"rate_limits":{"five_hour":{"utilization":72,"resets_at":"2026-10-08T00:40:00Z"},"seven_day":{"utilization":69,"resets_at":"2026-10-09T20:00:00Z"}},"session":{"model_usage":{}},"behaviors":null}`

func TestClaudeUsageRequiresVerifiedCompatibleVersion(t *testing.T) {
	for version, want := range map[string]bool{"2.1.293 (Claude Code)": true, "2.1.294 (Claude Code)": true, "2.1.289 (Claude Code)": false, "unknown": false, "2.1.293-beta": false} {
		if got := supportsClaudeUsage(version); got != want {
			t.Fatalf("version %q: %v", version, got)
		}
	}
}

func TestClaudeUsageControlReadsWithoutSendingModelPrompt(t *testing.T) {
	var output bytes.Buffer
	input := strings.NewReader("{\"type\":\"system\",\"subtype\":\"init\"}\n" + `{"type":"control_response","response":{"subtype":"success","request_id":"jonsbo-usage","response":` + fixtureClaudeUsage + `}}` + "\n")
	raw, err := claudeUsageExchange(input, &output)
	if err != nil {
		t.Fatal(err)
	}
	var request map[string]any
	if json.Unmarshal(output.Bytes(), &request) != nil || request["type"] != "control_request" || obj(request["request"])["subtype"] != "get_usage" || obj(request["request"])["skip_behaviors"] != true || request["message"] != nil {
		t.Fatalf("unsafe usage request: %s", output.Bytes())
	}
	o, err := parseClaudeQuotas(raw, "account", time.Now())
	if err != nil || o.Provider != data.Claude || o.Source != "claude-native-usage" || o.AuthMode != "subscription" || o.SessionID != "" || o.Tokens != nil || len(o.Quotas) != 2 || *o.Quotas[1].UsedPercent != 69 {
		t.Fatalf("wrong native quota: %+v %v", o, err)
	}
}

func TestClaudeUsageRejectsUnsupportedProtocolWithoutLeakingResponse(t *testing.T) {
	for _, input := range []string{
		`{"type":"control_response","response":{"subtype":"error","request_id":"jonsbo-usage","error":"secret body"}}`,
		`{"type":"assistant","message":"must never be requested"}`,
		`{"type":"control_response","response":{"subtype":"success","request_id":"unrelated","response":{}}}`,
		strings.Repeat("x", MaxPayload+1),
	} {
		_, err := claudeUsageExchange(strings.NewReader(input+"\n"), &bytes.Buffer{})
		if err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatalf("unsupported native usage or leaking error: %v", err)
		}
	}
}

type fixtureUsageRunner struct {
	fixtureRunner
	usageCalls    int
	statusCalls   int
	switchAccount bool
	logout        bool
	usageError    error
}

func (f *fixtureUsageRunner) ClaudeUsage(context.Context, string) ([]byte, error) {
	f.usageCalls++
	if f.usageError != nil {
		return nil, f.usageError
	}
	return []byte(fixtureClaudeUsage), nil
}
func (f *fixtureUsageRunner) Run(context.Context, string, []string) ([]byte, error) {
	f.statusCalls++
	if f.logout && f.statusCalls > 1 {
		return []byte(`{"loggedIn":false,"authMethod":"none"}`), nil
	}
	org := "org-a"
	if f.switchAccount && f.statusCalls > 1 {
		org = "org-b"
	}
	return []byte(`{"loggedIn":true,"authMethod":"claude.ai","email":"synthetic@example.test","orgId":"` + org + `","subscriptionType":"team"}`), nil
}

func TestClaudeBackgroundRefreshReadsNativeUsageWithoutLocalSessions(t *testing.T) {
	m, _, observations := testNativeManager(t)
	defer m.cancel()
	f := &fixtureUsageRunner{}
	m.runner = f
	m.enabled[data.Claude] = true
	if err := m.refresh(data.Claude); err != nil {
		t.Fatal(err)
	}
	if f.usageCalls != 1 || len(*observations) != 2 || (*observations)[1].Kind != "quota" || (*observations)[1].AccountKey != (*observations)[0].AccountKey || (*observations)[1].SessionID != "" {
		t.Fatalf("background usage missing: %+v", *observations)
	}
	if err := m.refresh(data.Claude); err != nil {
		t.Fatal(err)
	}
	if f.usageCalls != 1 {
		t.Fatal("native usage polled more frequently than the background interval")
	}
}

func TestClaudeUsageDoesNotCrossAccountSwitch(t *testing.T) {
	m, _, observations := testNativeManager(t)
	defer m.cancel()
	m.runner = &fixtureUsageRunner{switchAccount: true}
	m.enabled[data.Claude] = true
	if err := m.refresh(data.Claude); err == nil {
		t.Fatal("native account switch was not reported")
	}
	for _, o := range *observations {
		if len(o.Quotas) > 0 {
			t.Fatal("quota crossed accounts")
		}
	}
}

func TestClaudeUsageFinalLogoutDoesNotPublishConnectedAccount(t *testing.T) {
	m, _, observations := testNativeManager(t)
	defer m.cancel()
	m.runner = &fixtureUsageRunner{logout: true}
	m.enabled[data.Claude] = true
	if err := m.refresh(data.Claude); err == nil {
		t.Fatal("logout was not reported")
	}
	if len(*observations) != 1 || (*observations)[0].Connection != "reauthentication" || len((*observations)[0].Quotas) > 0 {
		t.Fatalf("logout kept old connected state: %+v", *observations)
	}
	if p := providerJSON(t, m, data.Claude); p["login"] != "signed-out" {
		t.Fatalf("logout status: %+v", p)
	}
}

func TestNativeQuotaFailureDoesNotExposeRawErrors(t *testing.T) {
	m, _, _ := testNativeManager(t)
	defer m.cancel()
	m.runner = &fixtureUsageRunner{usageError: errors.New("private token and user path MUST_NOT_LEAK")}
	m.enabled[data.Claude] = true
	if err := m.refresh(data.Claude); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(m.providers[data.Claude].Message, "MUST_NOT_LEAK") {
		t.Fatal("raw quota error exposed in status")
	}
}

func TestOptionalClaudeUsageFailureDoesNotBackOffAccountPolls(t *testing.T) {
	m, _, _ := testNativeManager(t)
	defer m.cancel()
	f := &fixtureUsageRunner{usageError: errors.New("background usage requires a newer Claude Code")}
	m.runner = f
	m.enabled[data.Claude] = true
	for i := 0; i < 2; i++ {
		if err := m.refresh(data.Claude); err != nil {
			t.Fatalf("optional quota error stopped account polling: %v", err)
		}
	}
	if f.usageCalls != 1 || m.providers[data.Claude].Connection != "connected" || !strings.Contains(m.providers[data.Claude].Message, "newer") {
		t.Fatal("optional usage retry or diagnostics lost")
	}
}

type timeoutUsageRunner struct{ fixtureUsageRunner }

func (f *timeoutUsageRunner) ClaudeUsage(ctx context.Context, _ string) ([]byte, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
func (f *timeoutUsageRunner) Run(ctx context.Context, path string, args []string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return f.fixtureUsageRunner.Run(ctx, path, args)
}
func TestClaudeQuotaTimeoutStillVerifiesHealthyAccount(t *testing.T) {
	m, _, observations := testNativeManager(t)
	defer m.cancel()
	m.runner = &timeoutUsageRunner{}
	m.enabled[data.Claude] = true
	m.claudeUsageTimeout = time.Millisecond
	if err := m.refresh(data.Claude); err != nil {
		t.Fatalf("quota timeout stopped authentication polling: %v", err)
	}
	if len(*observations) != 1 || (*observations)[0].Connection != "connected" {
		t.Fatal("quota timeout marked healthy account signed out")
	}
}

func TestClaudeQuotaRejectsMalformedAndUnknownWindows(t *testing.T) {
	for _, raw := range []string{`{"rate_limits_available":true,"rate_limits":{"seven_day":{"utilization":101}}}`, `{"rate_limits_available":true,"rate_limits":{"seven_day":{"utilization":"NaN"}}}`, `{"rate_limits_available":true,"rate_limits":{"secret":{"utilization":50}}}`, `null`} {
		if _, err := parseClaudeQuotas([]byte(raw), "a", time.Now()); err == nil {
			t.Fatalf("accepted unsupported quota: %s", raw)
		}
	}
}
