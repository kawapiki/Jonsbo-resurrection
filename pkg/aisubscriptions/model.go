// Package aisubscriptions contains the metadata-only interchange contract for
// subscription accounts, local agent observations, and display widgets.
package aisubscriptions

import "time"

const (
	OpenAI = "openai"
	Claude = "claude"
)

type Quota struct {
	ID            string     `json:"id"`
	UsedPercent   *float64   `json:"used_percent"`
	WindowMinutes *int64     `json:"window_minutes"`
	ResetsAt      *time.Time `json:"resets_at"`
	ObservedAt    time.Time  `json:"observed_at"`
}

type DailyBucket struct {
	Date   string `json:"date"`
	Tokens int64  `json:"tokens"`
}

type TokenUsage struct {
	Total           int64 `json:"total"`
	Input           int64 `json:"input"`
	Output          int64 `json:"output"`
	CachedInput     int64 `json:"cached_input"`
	CacheWrite      int64 `json:"cache_write"`
	ReasoningOutput int64 `json:"reasoning_output"`
}

type ContextUsage struct {
	Used        *int64   `json:"used"`
	Limit       *int64   `json:"limit"`
	UsedPercent *float64 `json:"used_percent"`
}

// Observation is a projected provider/native event. It never carries tokens,
// cookies, prompts, responses, executable commands or transcript paths.
type Observation struct {
	Kind       string        `json:"kind"`
	Provider   string        `json:"provider"`
	AccountKey string        `json:"account_key,omitempty"`
	AuthMode   string        `json:"auth_mode,omitempty"`
	Plan       string        `json:"plan,omitempty"`
	Connection string        `json:"connection,omitempty"`
	Source     string        `json:"source"`
	ObservedAt time.Time     `json:"observed_at"`
	SessionID  string        `json:"session_id,omitempty"`
	ProcessID  string        `json:"process_id,omitempty"`
	RequestID  string        `json:"request_id,omitempty"`
	Title      string        `json:"title,omitempty"`
	Model      string        `json:"model,omitempty"`
	Activity   string        `json:"activity,omitempty"`
	Tokens     *TokenUsage   `json:"tokens,omitempty"`
	Context    *ContextUsage `json:"context,omitempty"`
	Quotas     []Quota       `json:"quotas,omitempty"`
	DailyUsage []DailyBucket `json:"daily_usage,omitempty"`
	Complete   bool          `json:"complete,omitempty"`
}

type Capabilities struct {
	AccountQuota    bool `json:"account_quota"`
	AccountTokens   bool `json:"account_tokens"`
	LocalSessions   bool `json:"local_sessions"`
	BrowserActivity bool `json:"browser_activity"`
	ExactContext    bool `json:"exact_context"`
}

type Provider struct {
	ID           string    `json:"id"`
	AccountKey   string    `json:"account_key,omitempty"`
	AuthMode     string    `json:"auth_mode,omitempty"`
	Plan         string    `json:"plan,omitempty"`
	Connection   string    `json:"connection"`
	Source       string    `json:"source,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
	Stale        bool      `json:"stale"`
	Quotas       []Quota   `json:"quotas"`
	WeeklyTokens *int64    `json:"weekly_tokens"`
	// Local fields are observations without account attribution, never account totals.
	LocalWeeklyTokens *int64       `json:"local_weekly_tokens,omitempty"`
	LocalQuotas       []Quota      `json:"local_quotas,omitempty"`
	UsageLabel        string       `json:"usage_label"`
	Coverage          string       `json:"coverage"`
	Partial           bool         `json:"partial"`
	Capabilities      Capabilities `json:"capabilities"`
}

type Session struct {
	Provider       string        `json:"provider"`
	AccountKey     string        `json:"account_key,omitempty"`
	ID             string        `json:"id"`
	ProcessID      string        `json:"process_id,omitempty"`
	Source         string        `json:"source"`
	Title          string        `json:"title,omitempty"`
	Model          string        `json:"model,omitempty"`
	Activity       string        `json:"activity"`
	ObservedAt     time.Time     `json:"observed_at"`
	Stale          bool          `json:"stale"`
	ConsumedTokens *int64        `json:"consumed_tokens"`
	Context        *ContextUsage `json:"context"`
	LocalQuotas    []Quota       `json:"local_quotas,omitempty"`
}

type State struct {
	Providers []Provider `json:"providers"`
	Sessions  []Session  `json:"sessions"`
	UpdatedAt time.Time  `json:"updated_at"`
	Partial   bool       `json:"partial"`
}
