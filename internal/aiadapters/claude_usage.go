package aiadapters

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"

	data "github.com/kawapiki/Jonsbo-resurrection/pkg/aisubscriptions"
)

// Claude usage is read through the native client, which owns OAuth renewal.
// The experimental control request is the headless /usage surface. It sends no
// user/model message and explicitly excludes transcript-based behavior summaries.
func (n nativeRunner) ClaudeUsage(ctx context.Context, path string) ([]byte, error) {
	version, err := n.Run(ctx, path, []string{"--version"})
	if err != nil || !supportsClaudeUsage(string(version)) {
		return nil, errors.New("background Claude usage requires Claude Code 2.1.293 or later")
	}
	cmd := exec.CommandContext(ctx, path, "--safe-mode", "--no-session-persistence", "-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose")
	hide(cmd)
	for _, entry := range cleanEnvironment() {
		name := strings.ToUpper(strings.SplitN(entry, "=", 2)[0])
		switch name {
		case "DISABLE_TELEMETRY", "DISABLE_ERROR_REPORTING", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC", "DISABLE_AUTOUPDATER", "CLAUDE_CODE_ENABLE_TELEMETRY":
			continue
		}
		cmd.Env = append(cmd.Env, entry)
	}
	// Claude classifies quota reads as nonessential traffic. Disable optional
	// reporting individually so the explicitly requested usage read can run.
	cmd.Env = append(cmd.Env, "DISABLE_TELEMETRY=1", "DISABLE_ERROR_REPORTING=1", "DISABLE_AUTOUPDATER=1", "CLAUDE_CODE_ENABLE_TELEMETRY=0")
	cmd.Stderr = io.Discard
	input, err := cmd.StdinPipe()
	if err != nil {
		return nil, errors.New("cannot create native Claude usage input")
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		input.Close()
		return nil, errors.New("cannot create native Claude usage output")
	}
	if err = cmd.Start(); err != nil {
		input.Close()
		output.Close()
		return nil, errors.New("cannot start native Claude usage reader")
	}
	defer func() { input.Close(); cmd.Process.Kill(); cmd.Wait(); output.Close() }()
	return claudeUsageExchange(output, input)
}

func supportsClaudeUsage(version string) bool {
	fields := strings.Fields(version)
	if len(fields) == 0 {
		return false
	}
	parts := strings.Split(fields[0], ".")
	if len(parts) != 3 {
		return false
	}
	major, e1 := strconv.Atoi(parts[0])
	minor, e2 := strconv.Atoi(parts[1])
	patch, e3 := strconv.Atoi(parts[2])
	return e1 == nil && e2 == nil && e3 == nil && (major > 2 || major == 2 && (minor > 1 || minor == 1 && patch >= 293))
}

func claudeUsageExchange(input io.Reader, output io.Writer) ([]byte, error) {
	request := map[string]any{"type": "control_request", "request_id": "jonsbo-usage", "request": map[string]any{"subtype": "get_usage", "skip_behaviors": true}}
	if err := json.NewEncoder(output).Encode(request); err != nil {
		return nil, errors.New("native Claude usage request failed")
	}
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), MaxPayload+1)
	total := 0
	for scanner.Scan() {
		total += len(scanner.Bytes())
		if total > MaxPayload {
			return nil, errors.New("native Claude usage output exceeded limit")
		}
		var reply struct {
			Type     string `json:"type"`
			Response struct {
				Subtype   string          `json:"subtype"`
				RequestID string          `json:"request_id"`
				Data      json.RawMessage `json:"response"`
			} `json:"response"`
		}
		if json.Unmarshal(scanner.Bytes(), &reply) != nil {
			return nil, errors.New("invalid native Claude usage response")
		}
		if reply.Type == "system" {
			continue
		}
		if reply.Type != "control_response" {
			return nil, errors.New("unexpected native Claude usage message")
		}
		if reply.Response.RequestID != "jonsbo-usage" {
			continue
		}
		if reply.Response.Subtype != "success" || len(reply.Response.Data) == 0 {
			return nil, errors.New("native Claude background usage is unavailable; update Claude Code or sign in again")
		}
		return reply.Response.Data, nil
	}
	return nil, errors.New("native Claude usage response did not arrive")
}

func parseClaudeQuotas(raw []byte, account string, now time.Time) (data.Observation, error) {
	o := base(data.Claude, "claude-native-usage", "", now)
	o.Kind = "quota"
	o.AccountKey = account
	o.AuthMode = "subscription"
	m, err := object(raw)
	if err != nil {
		return o, err
	}
	available, _ := m["rate_limits_available"].(bool)
	if !available || m["behaviors"] != nil {
		return o, errors.New("native Claude subscription quota is unavailable")
	}
	limits := obj(m["rate_limits"])
	for _, window := range []struct {
		id      string
		minutes int64
	}{{"five_hour", 300}, {"seven_day", 10080}, {"seven_day_opus", 10080}, {"seven_day_sonnet", 10080}, {"seven_day_cowork", 10080}, {"seven_day_oauth_apps", 10080}} {
		value := obj(limits[window.id])
		p := percent(value["utilization"])
		if p == nil {
			continue
		}
		minutes := window.minutes
		q := data.Quota{ID: window.id, UsedPercent: p, WindowMinutes: &minutes, ObservedAt: now.UTC()}
		if reset := str(value["resets_at"]); reset != "" {
			at, e := time.Parse(time.RFC3339Nano, reset)
			if e != nil {
				return o, errors.New("invalid native Claude quota reset")
			}
			at = at.UTC()
			q.ResetsAt = &at
		}
		o.Quotas = append(o.Quotas, q)
	}
	if len(o.Quotas) == 0 {
		return o, errors.New("native Claude quota windows were not reported")
	}
	return o, nil
}
