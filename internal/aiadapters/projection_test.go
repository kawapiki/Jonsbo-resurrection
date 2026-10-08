package aiadapters

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestStatuslineContextIsNotConsumedUsage(t *testing.T) {
	raw := []byte(`{"session_id":"s","model":{"id":"opus"},"context_window":{"context_window_size":200000,"current_usage":{"input_tokens":100,"cache_read_input_tokens":200,"cache_creation_input_tokens":300},"used_percentage":1},"rate_limits":{"seven_day":{"used_percentage":42,"resets_at":1900000000}},"transcript_path":"secret","prompt":"private"}`)
	obs, err := ProjectNative("claude", "statusline", raw, time.Now())
	if err != nil || len(obs) != 2 {
		t.Fatalf("projection: %v %v", obs, err)
	}
	if obs[0].Kind != "context" || obs[0].Context.Used == nil || *obs[0].Context.Used != 600 || obs[0].Tokens != nil || obs[0].AccountKey != "" {
		t.Fatalf("incorrect context: %+v", obs[0])
	}
	encoded, _ := json.Marshal(obs)
	if strings.Contains(string(encoded), "private") || strings.Contains(string(encoded), "secret") {
		t.Fatal("content leaked")
	}
}

func TestDailyUsageRetainsOnlyLatest35ProviderDays(t *testing.T) {
	now := time.Date(2026, 10, 7, 20, 0, 0, 0, time.UTC)
	var buckets []string
	for i := 0; i < 40; i++ {
		buckets = append(buckets, fmt.Sprintf(`{"startDate":%q,"tokens":%d}`, now.AddDate(0, 0, -i).Format("2006-01-02"), 100+i))
	}
	raw := []byte(`{"dailyUsageBuckets":[` + strings.Join(buckets, ",") + `]}`)
	o, err := parseOpenAIUsage(raw, "a", now)
	if err != nil || len(o.DailyUsage) != 35 || o.DailyUsage[0].Date != "2026-09-03" || o.DailyUsage[34].Date != "2026-10-07" {
		t.Fatalf("%+v %v", o, err)
	}
}
func TestMetadataTitleTruncationPreservesUTF8(t *testing.T) {
	title := strings.Repeat("a", 255) + "ő"
	projected := str(title)
	if len(projected) > 256 || !utf8.ValidString(projected) {
		t.Fatalf("invalid truncated title %q", projected)
	}
}
func TestQuotaOrderingPrefersCodexLimit(t *testing.T) {
	raw := []byte(`{"rateLimitsByLimitId":{"z-extra":{"primary":{"usedPercent":20,"windowDurationMins":10080}},"codex":{"primary":{"usedPercent":50,"windowDurationMins":10080}}}}`)
	for i := 0; i < 20; i++ {
		o, err := parseOpenAIQuotas(raw, "a", time.Now())
		if err != nil || len(o.Quotas) != 2 || o.Quotas[0].ID != "codex:primary" {
			t.Fatalf("%+v %v", o, err)
		}
	}
}

func TestCompactionClearsContextAndHooksStayUnattributed(t *testing.T) {
	obs, err := ProjectNative("claude", "hook", []byte(`{"session_id":"s","hook_event_name":"PreCompact","account_key":"forged"}`), time.Now())
	if err != nil || len(obs) != 2 || obs[1].Context == nil || obs[1].Context.Used != nil || obs[0].AccountKey != "" {
		t.Fatalf("%+v %v", obs, err)
	}
}

func TestOTLPTokensRequireRequestIDAndClaudeCategoriesAdd(t *testing.T) {
	raw := []byte(`{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"attributes":[{"key":"event.name","value":{"stringValue":"api_request"}},{"key":"session.id","value":{"stringValue":"s"}},{"key":"request.id","value":{"stringValue":"r"}},{"key":"input_tokens","value":{"intValue":"10"}},{"key":"output_tokens","value":{"intValue":"20"}},{"key":"cache_read_tokens","value":{"intValue":"30"}},{"key":"cache_creation_tokens","value":{"intValue":"40"}},{"key":"prompt","value":{"stringValue":"private"}}]}]}]}]}`)
	obs, err := ProjectOTLP("claude", raw, time.Now())
	if err != nil || len(obs) != 1 || obs[0].Tokens == nil || obs[0].Tokens.Total != 100 || obs[0].RequestID != "r" || obs[0].AccountKey != "" {
		t.Fatalf("%+v %v", obs, err)
	}
	obs, err = ProjectOTLP("claude", []byte(strings.ReplaceAll(string(raw), `"request.id"`, `"unknown"`)), time.Now())
	if err != nil || len(obs) != 0 {
		t.Fatalf("missing identity accepted: %+v %v", obs, err)
	}
}

func TestAccountParsingRejectsAPIMode(t *testing.T) {
	obs, err := parseClaudeAccount([]byte(`{"loggedIn":true,"authMethod":"api_key","accountUuid":"a"}`), time.Now())
	if err == nil || obs.AccountKey != "" {
		t.Fatal("API account accepted")
	}
	obs, err = parseClaudeAccount([]byte(`{"loggedIn":true,"authMethod":"claude.ai","accountUuid":"a","subscriptionType":"max"}`), time.Now())
	if err != nil || obs.AuthMode != "subscription" || obs.AccountKey == "" || obs.AccountKey == "a" {
		t.Fatalf("%+v %v", obs, err)
	}
}

func TestTelemetryPreservesEventTimeAndOpenAICacheIsIncluded(t *testing.T) {
	now := time.Date(2026, 10, 7, 20, 0, 0, 0, time.UTC)
	raw := []byte(`{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"attributes":[{"key":"event.name","value":{"stringValue":"codex.sse_event"}},{"key":"conversation.id","value":{"stringValue":"s"}},{"key":"response.id","value":{"stringValue":"r"}},{"key":"event.timestamp","value":{"stringValue":"2026-10-06T20:00:00Z"}},{"key":"input_tokens","value":{"intValue":"100"}},{"key":"output_tokens","value":{"intValue":"20"}},{"key":"cached_input_tokens","value":{"intValue":"50"}}]}]}]}]}`)
	obs, err := ProjectOTLP("openai", raw, now)
	if err != nil || len(obs) != 1 || obs[0].Tokens.Total != 120 || !obs[0].ObservedAt.Equal(now.Add(-24*time.Hour)) {
		t.Fatalf("%+v %v", obs, err)
	}
}
