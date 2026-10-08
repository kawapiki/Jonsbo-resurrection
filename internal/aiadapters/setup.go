package aiadapters

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
)

// setup is a merge proposal only. It never edits native settings or hook trust.
func (m *Manager) setup(r *http.Request) map[string]any {
	base := "http://" + r.Host
	tokenPath := filepath.Join(m.dir, "observer.token")
	helper := "jonsbo-ai-observer"
	command := func(provider, mode string) string {
		return helper + " " + mode + " --provider " + provider + " --api " + strconv.Quote(base) + " --token-file " + strconv.Quote(tokenPath)
	}
	hooks := func(provider string) map[string]any {
		out := map[string]any{}
		for _, event := range []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PermissionRequest", "Stop", "SessionEnd", "PreCompact"} {
			timeout := 5
			if event == "SessionEnd" {
				timeout = 3
			}
			out[event] = []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": command(provider, "hook"), "timeout": timeout}}}}
		}
		return map[string]any{"hooks": out}
	}
	claude := hooks("claude")
	claude["statusLine"] = map[string]any{"type": "command", "command": command("claude", "statusline")}
	return map[string]any{
		"instructions":        []string{"Build cmd/jonsbo-ai-observer and place its executable on PATH, or replace the helper name with its absolute quoted path.", "Merge hook arrays with existing native hooks; do not replace them. Review and approve Codex hook trust in the native client.", "Use scripts/configure-claude-observer.ps1 to configure local Claude status-line and telemetry forwarding. It preserves other settings and backs up the settings file; an existing unrelated status line or OTLP destination requires manual merging.", "Merge exporter configuration with existing exporters. Headers contain a scoped observer credential and must stay local. Do not enable prompt, response, or tool content export.", "Hook/status-line observations remain local until a feed establishes that session's own subscription identity. A global login does not establish origin.", "Subscription quotas refresh in the background without an active terminal. Claude requires Code 2.1.293 or later and uses native headless usage with no model message or transcript behavior scan. Status-line quotas are a local fallback. Claude weekly request tokens are local observations, separate from linked-account totals; account-wide Claude token history remains unavailable.", "Native ingress accepts no browser Origin header. Pair the browser native host with browser.token; content scripts never receive the controller credential."},
		"observer_token_file": tokenPath, "browser_token_file": filepath.Join(m.dir, "browser.token"), "claude_settings": claude, "codex_hooks": hooks("openai"),
		"codex_otel_toml": fmt.Sprintf("[otel]\nlog_user_prompt = false\n\n[otel.exporter.otlp-http]\nendpoint = %s\nprotocol = \"json\"\n\n[otel.exporter.otlp-http.headers]\nAuthorization = %s\n", strconv.Quote(base+"/v1/ai/telemetry/openai/v1/logs"), strconv.Quote("Bearer "+m.observerToken)),
		"claude_otel_env": map[string]string{"CLAUDE_CODE_ENABLE_TELEMETRY": "1", "OTEL_LOGS_EXPORTER": "otlp", "OTEL_EXPORTER_OTLP_LOGS_PROTOCOL": "http/json", "OTEL_EXPORTER_OTLP_LOGS_ENDPOINT": base + "/v1/ai/telemetry/claude/v1/logs", "OTEL_EXPORTER_OTLP_LOGS_HEADERS": "Authorization=Bearer " + m.observerToken, "OTEL_LOG_USER_PROMPTS": "0", "OTEL_LOG_ASSISTANT_RESPONSES": "0", "OTEL_LOG_TOOL_DETAILS": "0", "OTEL_LOG_TOOL_CONTENT": "0", "OTEL_LOG_RAW_API_BODIES": "0"},
	}
}
