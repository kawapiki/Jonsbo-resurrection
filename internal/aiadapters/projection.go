// Package aiadapters projects native metadata into the subscription contract.
// Raw native payloads are never persisted or forwarded by this package.
package aiadapters

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	data "github.com/kawapiki/Jonsbo-resurrection/pkg/aisubscriptions"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const MaxPayload = 1 << 20
const maxTokens int64 = 1_000_000_000_000

func object(raw []byte) (map[string]any, error) {
	if len(raw) > MaxPayload {
		return nil, errors.New("metadata payload exceeds 1 MiB")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var m map[string]any
	if err := d.Decode(&m); err != nil || m == nil {
		return nil, errors.New("invalid metadata JSON")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return nil, errors.New("multiple JSON values")
	}
	return m, nil
}
func str(v any) string {
	s, _ := v.(string)
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	if len(s) > 256 {
		s = s[:256]
		for !utf8.ValidString(s) {
			s = s[:len(s)-1]
		}
	}
	return s
}
func obj(v any) map[string]any { m, _ := v.(map[string]any); return m }
func list(v any) []any         { a, _ := v.([]any); return a }
func number(v any) *int64 {
	var n int64
	var err error
	switch x := v.(type) {
	case json.Number:
		n, err = x.Int64()
	case string:
		n, err = strconv.ParseInt(x, 10, 64)
	default:
		return nil
	}
	if err != nil || n < 0 || n > maxTokens {
		return nil
	}
	return &n
}
func percent(v any) *float64 {
	var f float64
	var err error
	switch x := v.(type) {
	case json.Number:
		f, err = x.Float64()
	case string:
		f, err = strconv.ParseFloat(x, 64)
	default:
		return nil
	}
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || f < 0 || f > 100 {
		return nil
	}
	return &f
}
func val(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
func opaque(provider, id string) string {
	h := sha256.Sum256([]byte(provider + ":" + id))
	return hex.EncodeToString(h[:16])
}
func base(provider, source, session string, now time.Time) data.Observation {
	return data.Observation{Provider: provider, Source: source, SessionID: session, AuthMode: "unknown", ObservedAt: now.UTC()}
}

// ProjectNative accepts hook/status-line JSON and selects metadata only. A global
// login cannot establish a pre-existing session's identity, so these stay local.
func ProjectNative(provider, mode string, raw []byte, now time.Time) ([]data.Observation, error) {
	if provider != data.OpenAI && provider != data.Claude {
		return nil, errors.New("unknown provider")
	}
	m, err := object(raw)
	if err != nil {
		return nil, err
	}
	sid := str(m["session_id"])
	if sid == "" {
		return nil, errors.New("session_id is required")
	}
	source := "codex-hook"
	if provider == data.Claude {
		source = "claude-hook"
	}
	o := base(provider, source, sid, now)
	o.ProcessID = str(m["process_instance_id"])
	if mode == "statusline" {
		if provider != data.Claude {
			return nil, errors.New("statusline is supported by Claude Code")
		}
		o.Source = "claude-statusline"
		o.Kind = "context"
		o.Model = str(obj(m["model"])["id"])
		o.Title = str(m["session_name"])
		o.Context = &data.ContextUsage{}
		c := obj(m["context_window"])
		o.Context.Limit = number(c["context_window_size"])
		o.Context.UsedPercent = percent(c["used_percentage"])
		if u := obj(c["current_usage"]); u != nil {
			input, read, write := number(u["input_tokens"]), number(u["cache_read_input_tokens"]), number(u["cache_creation_input_tokens"])
			if input != nil && read != nil && write != nil {
				n := *input + *read + *write
				o.Context.Used = &n
			}
		}
		out := []data.Observation{o}
		var quotas []data.Quota
		for _, w := range []struct {
			k        string
			duration int64
		}{{"five_hour", 300}, {"seven_day", 10080}} {
			q := obj(obj(m["rate_limits"])[w.k])
			p := percent(q["used_percentage"])
			if p == nil {
				continue
			}
			d := w.duration
			quota := data.Quota{ID: w.k, UsedPercent: p, WindowMinutes: &d, ObservedAt: now.UTC()}
			if n := number(q["resets_at"]); n != nil {
				t := time.Unix(*n, 0).UTC()
				quota.ResetsAt = &t
			}
			quotas = append(quotas, quota)
		}
		if len(quotas) > 0 {
			q := base(provider, o.Source, sid, now)
			q.Kind = "quota"
			q.Quotas = quotas
			out = append(out, q)
		}
		return out, nil
	}
	if mode != "hook" {
		return nil, errors.New("unknown observer mode")
	}
	o.Kind = "session"
	o.Model = str(m["model"])
	event := str(m["hook_event_name"])
	switch event {
	case "SessionStart", "Stop", "Interrupt":
		o.Activity = "idle"
	case "SessionEnd":
		o.Activity = "completed"
	case "UserPromptSubmit", "PreToolUse", "PostToolUse", "SubagentStart":
		o.Activity = "running"
	case "PermissionRequest", "Notification":
		o.Activity = "waiting"
	case "PreCompact", "PostCompact":
		o.Activity = "unknown"
	default:
		return nil, nil
	}
	out := []data.Observation{o}
	if event == "PreCompact" || event == "PostCompact" {
		c := base(provider, source, sid, now)
		c.Kind = "context"
		c.Context = &data.ContextUsage{}
		out = append(out, c)
	}
	return out, nil
}

// ProjectOTLP supports bounded OTLP HTTP JSON logs, not protobuf or transcripts.
// Consumption is accepted only with a stable provider request identity.
func ProjectOTLP(provider string, raw []byte, now time.Time) ([]data.Observation, error) {
	if provider != data.OpenAI && provider != data.Claude {
		return nil, errors.New("unknown provider")
	}
	m, err := object(raw)
	if err != nil {
		return nil, err
	}
	var out []data.Observation
	for _, resource := range list(m["resourceLogs"]) {
		r := obj(resource)
		ra := attributes(r["resource"])
		for _, scope := range list(r["scopeLogs"]) {
			for _, record := range list(obj(scope)["logRecords"]) {
				if len(out) >= 256 {
					return nil, errors.New("too many telemetry records")
				}
				a := make(map[string]any)
				for k, v := range ra {
					a[k] = v
				}
				for k, v := range attributes(record) {
					a[k] = v
				}
				event := str(a["event.name"])
				if event == "" {
					event = str(a["event_name"])
				}
				sid := first(a, "session.id", "conversation.id", "thread.id")
				rid := first(a, "request.id", "request_id", "response.id", "response_id")
				if sid == "" || rid == "" {
					continue
				}
				if provider == data.Claude && event != "api_request" && event != "claude_code.api_request" {
					continue
				}
				if provider == data.OpenAI && event != "codex.sse_event" && event != "codex.api_request" && event != "codex.request" {
					continue
				}
				in := number(a["input_tokens"])
				output := number(a["output_tokens"])
				cached := number(a["cache_read_tokens"])
				if cached == nil {
					cached = number(a["cached_input_tokens"])
				}
				write := number(a["cache_creation_tokens"])
				if write == nil {
					write = number(a["cache_write_input_tokens"])
				}
				total := number(a["total_tokens"])
				if in == nil && output == nil && cached == nil && write == nil && total == nil {
					continue
				}
				t := data.TokenUsage{Input: val(in), Output: val(output), CachedInput: val(cached), CacheWrite: val(write), ReasoningOutput: val(number(a["reasoning_output_tokens"]))}
				if total != nil {
					t.Total = *total
				} else {
					t.Total = t.Input + t.Output
					if provider == data.Claude {
						t.Total += t.CachedInput + t.CacheWrite
					}
				}
				source := "codex-otel"
				if provider == data.Claude {
					source = "claude-otel"
				}
				observed := now
				if timestamp := str(a["event.timestamp"]); timestamp != "" {
					if parsed, e := time.Parse(time.RFC3339Nano, timestamp); e == nil {
						observed = parsed
					}
				} else if nanos := obj(record)["timeUnixNano"]; nanos != nil {
					text := str(nanos)
					if n, ok := nanos.(json.Number); ok {
						text = n.String()
					}
					if n, e := strconv.ParseInt(text, 10, 64); e == nil && n > 0 {
						observed = time.Unix(0, n)
					}
				}
				if observed.After(now.Add(time.Minute)) || observed.Before(now.Add(-35*24*time.Hour)) {
					continue
				}
				o := base(provider, source, sid, observed)
				o.Kind = "usage"
				o.RequestID = rid
				o.Model = first(a, "model", "model.name")
				o.Tokens = &t
				out = append(out, o)
			}
		}
	}
	return out, nil
}
func first(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s := str(m[k]); s != "" {
			return s
		}
	}
	return ""
}
func attributes(v any) map[string]any {
	m := obj(v)
	a := map[string]any{}
	for _, entry := range list(m["attributes"]) {
		e := obj(entry)
		k := str(e["key"])
		v := obj(e["value"])
		for _, field := range []string{"stringValue", "intValue", "doubleValue", "boolValue"} {
			if x, ok := v[field]; ok {
				a[k] = x
				break
			}
		}
	}
	return a
}

func parseClaudeAccount(raw []byte, now time.Time) (data.Observation, error) {
	m, err := object(raw)
	o := base(data.Claude, "claude-auth", "", now)
	o.Kind = "account"
	o.Connection = "reauthentication"
	if err != nil {
		return o, err
	}
	logged, _ := m["loggedIn"].(bool)
	if !logged {
		return o, errors.New("Claude Code subscription sign-in required")
	}
	auth := str(m["authMethod"])
	if auth != "claude.ai" {
		return o, errors.New("Claude Code must use subscription authentication (claude.ai)")
	}
	id := first(m, "accountUuid", "accountUUID")
	if id == "" {
		id = str(m["email"])
	}
	if id == "" {
		return o, errors.New("Claude Code did not return an account identity; update the native CLI")
	}
	if org := first(m, "orgId", "organizationUuid", "organizationUUID"); org != "" {
		id += ":" + org
	}
	o.AccountKey = opaque(data.Claude, id)
	o.AuthMode = "subscription"
	o.Plan = str(m["subscriptionType"])
	o.Connection = "connected"
	return o, nil
}

func parseOpenAIAccount(raw []byte, now time.Time) (data.Observation, error) {
	m, err := object(raw)
	o := base(data.OpenAI, "openai-app-server", "", now)
	o.Kind = "account"
	o.Connection = "reauthentication"
	if err != nil {
		return o, err
	}
	a := obj(m["account"])
	if str(a["type"]) != "chatgpt" {
		return o, errors.New("Codex ChatGPT subscription sign-in required")
	}
	id := first(a, "accountId", "email")
	if id == "" {
		return o, errors.New("Codex did not return a verified account identity")
	}
	o.AccountKey = opaque(data.OpenAI, id)
	o.AuthMode = "subscription"
	o.Plan = str(a["planType"])
	o.Connection = "connected"
	return o, nil
}

func parseOpenAIQuotas(raw []byte, account string, now time.Time) (data.Observation, error) {
	m, err := object(raw)
	o := base(data.OpenAI, "openai-app-server", "", now)
	o.Kind = "quota"
	o.AccountKey = account
	o.AuthMode = "subscription"
	if err != nil {
		return o, err
	}
	buckets := obj(m["rateLimitsByLimitId"])
	if len(buckets) == 0 {
		b := obj(m["rateLimits"])
		buckets = map[string]any{"codex": b}
	}
	keys := make([]string, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i] == keys[j] {
			return false
		}
		if keys[i] == "codex" {
			return true
		}
		if keys[j] == "codex" {
			return false
		}
		return keys[i] < keys[j]
	})
	for _, key := range keys {
		v := buckets[key]
		b := obj(v)
		for _, name := range []string{"primary", "secondary"} {
			w := obj(b[name])
			p := percent(w["usedPercent"])
			if p == nil {
				continue
			}
			q := data.Quota{ID: fmt.Sprintf("%s:%s", str(key), name), UsedPercent: p, WindowMinutes: number(w["windowDurationMins"]), ObservedAt: now.UTC()}
			if n := number(w["resetsAt"]); n != nil {
				t := time.Unix(*n, 0).UTC()
				q.ResetsAt = &t
			}
			o.Quotas = append(o.Quotas, q)
		}
	}
	return o, nil
}
func parseOpenAIUsage(raw []byte, account string, now time.Time) (data.Observation, error) {
	m, err := object(raw)
	o := base(data.OpenAI, "openai-app-server", "", now)
	o.Kind = "usage"
	o.AccountKey = account
	o.AuthMode = "subscription"
	if err != nil {
		return o, err
	}
	cutoff := now.UTC().AddDate(0, 0, -34).Format("2006-01-02")
	latest := now.UTC().Add(24 * time.Hour).Format("2006-01-02")
	buckets := map[string]int64{}
	for _, v := range list(m["dailyUsageBuckets"]) {
		b := obj(v)
		date := str(b["startDate"])
		n := number(b["tokens"])
		if _, err := time.Parse("2006-01-02", date); err == nil && n != nil && date >= cutoff && date <= latest {
			buckets[date] = *n
		}
	}
	dates := make([]string, 0, len(buckets))
	for date := range buckets {
		dates = append(dates, date)
	}
	sort.Strings(dates)
	if len(dates) > 35 {
		dates = dates[len(dates)-35:]
	}
	for _, date := range dates {
		o.DailyUsage = append(o.DailyUsage, data.DailyBucket{Date: date, Tokens: buckets[date]})
	}
	return o, nil
}

// NativeMetadata selects the native input fields needed by the projector before
// the observer helper sends anything to loopback. It preserves no source paths.
func NativeMetadata(provider, mode string, raw []byte) ([]byte, error) {
	if _, err := ProjectNative(provider, mode, raw, time.Now()); err != nil {
		return nil, err
	}
	m, err := object(raw)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"session_id": str(m["session_id"])}
	if mode == "hook" {
		out["hook_event_name"] = str(m["hook_event_name"])
		out["process_instance_id"] = str(m["process_instance_id"])
		out["model"] = str(m["model"])
	} else {
		out["session_name"] = str(m["session_name"])
		out["model"] = map[string]any{"id": str(obj(m["model"])["id"])}
		c := obj(m["context_window"])
		u := obj(c["current_usage"])
		context := map[string]any{"context_window_size": number(c["context_window_size"]), "used_percentage": percent(c["used_percentage"])}
		if u != nil {
			context["current_usage"] = map[string]any{"input_tokens": number(u["input_tokens"]), "cache_read_input_tokens": number(u["cache_read_input_tokens"]), "cache_creation_input_tokens": number(u["cache_creation_input_tokens"])}
		}
		out["context_window"] = context
		q := map[string]any{}
		for _, k := range []string{"five_hour", "seven_day"} {
			w := obj(obj(m["rate_limits"])[k])
			if w != nil {
				q[k] = map[string]any{"used_percentage": percent(w["used_percentage"]), "resets_at": number(w["resets_at"])}
			}
		}
		out["rate_limits"] = q
	}
	return json.Marshal(out)
}
