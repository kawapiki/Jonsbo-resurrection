// Package aisubscriptions aggregates bounded, metadata-only subscription feeds.
package aisubscriptions

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	_ "time/tzdata"
	"unicode"
	"unicode/utf8"

	data "github.com/kawapiki/Jonsbo-resurrection/pkg/aisubscriptions"
	"github.com/kawapiki/Jonsbo-resurrection/pkg/module"
)

const maxStorageBytes = 32 << 20
const maxSessions = 128
const maxDedup = 200000
const historyDays = 35

type accountRecord struct {
	Provider    data.Provider    `json:"provider"`
	Daily       map[string]int64 `json:"daily,omitempty"`
	AuthorityAt time.Time        `json:"authority_at,omitempty"`
	Complete    bool             `json:"complete"`
}
type diskState struct {
	Version    int                         `json:"version"`
	Accounts   map[string]accountRecord    `json:"accounts"`
	Linked     map[string]string           `json:"linked"`
	Sessions   map[string]data.Session     `json:"sessions"`
	ContextAt  map[string]time.Time        `json:"context_at"`
	Daily      map[string]map[string]int64 `json:"daily"`
	LocalDaily map[string]map[string]int64 `json:"local_daily,omitempty"`
	Dedup      map[string]time.Time        `json:"dedup"`
	Partial    bool                        `json:"partial"`
	UpdatedAt  time.Time                   `json:"updated_at"`
}
type Module struct {
	mu              sync.RWMutex
	state           diskState
	dir             string
	now             func() time.Time
	running, closed bool
	done            chan struct{}
}

var _ module.Module = (*Module)(nil)
var _ module.Renderer = (*Module)(nil)
var _ module.EventHandler = (*Module)(nil)

func New(dir string) (*Module, error) {
	m := &Module{dir: dir, now: time.Now, done: make(chan struct{}), state: diskState{Version: 1, Accounts: map[string]accountRecord{}, Linked: map[string]string{}, Sessions: map[string]data.Session{}, ContextAt: map[string]time.Time{}, Daily: map[string]map[string]int64{}, Dedup: map[string]time.Time{}}}
	if dir == "" {
		return m, nil
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	f, err := os.Open(filepath.Join(dir, "state-v1.json"))
	if os.IsNotExist(err) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxStorageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxStorageBytes {
		return nil, fmt.Errorf("%w: subscription storage exceeds limit", module.ErrInvalid)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m.state); err != nil {
		return nil, fmt.Errorf("subscription storage: %w", err)
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("%w: trailing storage JSON", module.ErrInvalid)
	}
	if m.state.Version != 1 || m.state.Accounts == nil || m.state.Linked == nil || m.state.Sessions == nil || m.state.Daily == nil || m.state.Dedup == nil || len(m.state.Sessions) > maxSessions || len(m.state.ContextAt) > maxSessions || len(m.state.Dedup) > maxDedup || len(m.state.Accounts) > 128 {
		return nil, fmt.Errorf("%w: subscription storage schema or bounds", module.ErrInvalid)
	}
	if m.state.ContextAt == nil {
		m.state.ContextAt = map[string]time.Time{}
	}
	if m.state.LocalDaily == nil {
		m.state.LocalDaily = map[string]map[string]int64{}
	}
	if len(m.state.LocalDaily) > 2 {
		return nil, fmt.Errorf("%w: local history bounds", module.ErrInvalid)
	}
	return m, nil
}
func (*Module) Descriptor() module.Descriptor {
	return module.Descriptor{ID: "ai-subscriptions", Name: "AI subscriptions", Version: "1.0.0", Description: "Subscription quotas and observed local/browser activity; unavailable fields remain unknown.", Views: []module.View{{ID: "overview", Width: 640, Height: 480}, {ID: "openai", Width: 640, Height: 180}, {ID: "claude", Width: 640, Height: 180}, {ID: "sessions", Width: 640, Height: 480}}}
}
func (m *Module) Run(ctx context.Context, publish module.Publish) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if publish == nil {
		return fmt.Errorf("%w: nil publisher", module.ErrInvalid)
	}
	m.mu.Lock()
	if m.running || m.closed {
		m.mu.Unlock()
		return module.ErrUnavailable
	}
	m.running = true
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.running = false
		if !m.closed {
			m.closed = true
			close(m.done)
		}
		m.mu.Unlock()
	}()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := publish(m.Snapshot()); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-m.done:
			return nil
		case <-ticker.C:
		}
	}
}

// Close detaches this module. Adapter ownership remains with the controller.
func (m *Module) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.closed {
		m.closed = true
		close(m.done)
	}
	return nil
}
func (m *Module) HandleEvent(ctx context.Context, e module.Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if e.Type != "observation" {
		return module.ErrUnsupported
	}
	if err := module.ValidateEvent(e); err != nil {
		return err
	}
	if !utf8.Valid(e.Payload) {
		return fmt.Errorf("%w: UTF-8", module.ErrInvalid)
	}
	var o data.Observation
	dec := json.NewDecoder(bytes.NewReader(e.Payload))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&o); err != nil {
		return fmt.Errorf("%w: observation JSON", module.ErrInvalid)
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("%w: trailing JSON", module.ErrInvalid)
	}
	return m.Observe(ctx, o)
}
func key(parts ...string) string { b, _ := json.Marshal(parts); return string(b) }
func member(s string, values ...string) bool {
	for _, v := range values {
		if s == v {
			return true
		}
	}
	return false
}
func boundedText(s string, n int) bool {
	return len(s) <= n && utf8.ValidString(s) && strings.IndexFunc(s, func(r rune) bool { return unicode.IsControl(r) }) < 0
}
func finitePercent(p *float64) bool {
	return p == nil || (!math.IsNaN(*p) && !math.IsInf(*p, 0) && *p >= 0 && *p <= 100)
}
func tokenTotal(t data.TokenUsage, provider string) (int64, error) {
	values := []int64{t.Total, t.Input, t.Output, t.CachedInput, t.CacheWrite, t.ReasoningOutput}
	for _, n := range values {
		if n < 0 {
			return 0, module.ErrInvalid
		}
	}
	if t.Total > 0 {
		return t.Total, nil
	}
	n := t.Input
	if math.MaxInt64-n < t.Output {
		return 0, module.ErrInvalid
	}
	n += t.Output
	if provider == data.Claude {
		for _, v := range []int64{t.CachedInput, t.CacheWrite} {
			if math.MaxInt64-n < v {
				return 0, module.ErrInvalid
			}
			n += v
		}
	}
	return n, nil
}
func validate(o data.Observation, now time.Time) error {
	invalid := func() error { return fmt.Errorf("%w: subscription observation", module.ErrInvalid) }
	if !member(o.Kind, "account", "session", "usage", "quota", "context", "disconnect") || !member(o.Provider, data.OpenAI, data.Claude) || !member(o.Source, "openai-app-server", "codex-hook", "codex-otel", "claude-auth", "claude-native-usage", "claude-hook", "claude-statusline", "claude-otel", "browser") {
		return invalid()
	}
	if o.Provider == data.OpenAI && strings.HasPrefix(o.Source, "claude-") || o.Provider == data.Claude && (strings.HasPrefix(o.Source, "codex-") || o.Source == "openai-app-server") {
		return invalid()
	}
	if !member(o.AuthMode, "", "subscription", "api", "unknown") || !member(o.Connection, "", "disconnected", "connecting", "connected", "reauthentication", "unavailable") || !member(o.Activity, "", "running", "waiting", "idle", "completed", "unknown") {
		return invalid()
	}
	if o.ObservedAt.IsZero() || o.ObservedAt.After(now.Add(5*time.Minute)) || o.ObservedAt.Before(now.AddDate(0, 0, -historyDays)) {
		return invalid()
	}
	for _, s := range []string{o.AccountKey, o.SessionID, o.ProcessID, o.RequestID, o.Model} {
		if !boundedText(s, 256) {
			return invalid()
		}
	}
	if !boundedText(o.Title, 512) || !boundedText(o.Plan, 128) || len(o.Quotas) > 32 || len(o.DailyUsage) > historyDays {
		return invalid()
	}
	if o.Kind == "account" && (o.Connection == "" || (o.Connection == "connected" && o.AccountKey == "") || o.Source == "browser") {
		return invalid()
	}
	if member(o.Kind, "session", "context") && o.SessionID == "" {
		return invalid()
	}
	if o.Kind == "usage" && o.Tokens == nil && len(o.DailyUsage) == 0 {
		return invalid()
	}
	if o.Tokens != nil {
		if o.Kind != "usage" || o.RequestID == "" {
			return invalid()
		}
		if _, err := tokenTotal(*o.Tokens, o.Provider); err != nil {
			return invalid()
		}
	}
	if o.Context != nil {
		c := o.Context
		if !finitePercent(c.UsedPercent) || c.Used != nil && *c.Used < 0 || c.Limit != nil && *c.Limit <= 0 {
			return invalid()
		}
	}
	for _, q := range o.Quotas {
		if !boundedText(q.ID, 128) || q.ID == "" || !finitePercent(q.UsedPercent) || q.WindowMinutes != nil && *q.WindowMinutes <= 0 || q.ObservedAt.IsZero() || q.ObservedAt.After(now.Add(5*time.Minute)) {
			return invalid()
		}
	}
	dates := map[string]bool{}
	for _, d := range o.DailyUsage {
		parsed, err := time.Parse("2006-01-02", d.Date)
		if err != nil || d.Tokens < 0 || dates[d.Date] || parsed.Before(now.AddDate(0, 0, -historyDays-1)) || d.Date > now.Add(24*time.Hour).Format("2006-01-02") {
			return invalid()
		}
		dates[d.Date] = true
	}
	b, err := json.Marshal(o)
	if err != nil || len(b) > module.MaxEventBytes {
		return invalid()
	}
	return nil
}
func clone[T any](v T) T { b, _ := json.Marshal(v); var c T; _ = json.Unmarshal(b, &c); return c }
func (m *Module) Observe(ctx context.Context, o data.Observation) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return module.ErrUnavailable
	}
	now := m.now()
	if err := validate(o, now); err != nil {
		return err
	}
	next := clone(m.state)
	if next.LocalDaily == nil {
		next.LocalDaily = map[string]map[string]int64{}
	}
	prune(&next, now)
	// An adapter can report lost authentication without knowing an identity.
	// Preserve the linked account's cached values until an explicit disconnect.
	if o.Kind == "account" && o.AccountKey == "" && o.Connection != "connected" {
		if linked, ok := next.Linked[o.Provider]; ok && linked != "" {
			previous := next.Accounts[key(o.Provider, linked)]
			o.AccountKey = linked
			o.AuthMode = previous.Provider.AuthMode
			o.Plan = previous.Provider.Plan
		}
	}
	accountKey := key(o.Provider, o.AccountKey)
	record, known := next.Accounts[accountKey]
	if o.Kind != "account" && o.Kind != "disconnect" && o.AccountKey != "" && (!known || o.AuthMode != "" && o.AuthMode != record.Provider.AuthMode) {
		return fmt.Errorf("%w: unverified account attribution", module.ErrInvalid)
	}
	if o.Kind == "disconnect" {
		if o.AccountKey == "" {
			o.AccountKey = next.Linked[o.Provider]
		}
		accountKey = key(o.Provider, o.AccountKey)
		delete(next.Accounts, accountKey)
		delete(next.Daily, accountKey)
		for k, s := range next.Sessions {
			if s.Provider == o.Provider && s.AccountKey == o.AccountKey {
				delete(next.Sessions, k)
				delete(next.ContextAt, k)
			}
		}
		if next.Linked[o.Provider] == o.AccountKey {
			delete(next.Linked, o.Provider)
		}
	} else {
		if o.Kind == "account" {
			if !known && len(next.Accounts) >= 128 {
				return m.limit(ctx, "account limit")
			}
			if !known {
				record.Provider = data.Provider{ID: o.Provider, AccountKey: o.AccountKey, Quotas: []data.Quota{}}
			}
			if !o.ObservedAt.Before(record.Provider.UpdatedAt) {
				record.Provider.AuthMode = o.AuthMode
				if o.AuthMode == "" {
					record.Provider.AuthMode = "unknown"
				}
				record.Provider.Plan = o.Plan
				record.Provider.Connection = o.Connection
				record.Provider.Source = o.Source
				record.Provider.UpdatedAt = o.ObservedAt
				linkedAccount := next.Accounts[key(o.Provider, next.Linked[o.Provider])]
				if !o.ObservedAt.Before(linkedAccount.Provider.UpdatedAt) {
					next.Linked[o.Provider] = o.AccountKey
				}
			}
		}
		if known || o.Kind == "account" {
			if len(o.Quotas) > 0 && !o.ObservedAt.Before(record.Provider.UpdatedAt.Add(-time.Second)) {
				record.Provider.Quotas = clone(o.Quotas)
				record.Provider.Capabilities.AccountQuota = true
			}
			if len(o.DailyUsage) > 0 && !o.ObservedAt.Before(record.AuthorityAt) {
				record.Daily = map[string]int64{}
				for _, d := range o.DailyUsage {
					record.Daily[d.Date] = d.Tokens
				}
				record.AuthorityAt = o.ObservedAt
				record.Complete = o.Complete
				record.Provider.Capabilities.AccountTokens = true
			}
			next.Accounts[accountKey] = record
		}
		if o.SessionID != "" {
			sk := key(o.Provider, o.AccountKey, o.SessionID)
			s, exists := next.Sessions[sk]
			closedBrowser := o.Kind == "session" && o.Source == "browser" && o.Activity == "completed"
			if closedBrowser {
				if exists && !o.ObservedAt.Before(s.ObservedAt) {
					delete(next.Sessions, sk)
					delete(next.ContextAt, sk)
				}
			} else {
				if !exists && len(next.Sessions) >= maxSessions && !reclaimSession(&next, now) {
					return m.limit(ctx, "session limit")
				}
				if !exists {
					s = data.Session{Provider: o.Provider, AccountKey: o.AccountKey, ID: o.SessionID, Source: o.Source, Activity: "unknown"}
				}
				if !o.ObservedAt.Before(s.ObservedAt) {
					s.Source = o.Source
					s.ObservedAt = o.ObservedAt
					if o.ProcessID != "" {
						s.ProcessID = o.ProcessID
					}
					if o.Title != "" {
						s.Title = o.Title
					}
					if o.Model != "" {
						s.Model = o.Model
					}
					if o.Activity != "" {
						s.Activity = o.Activity
					}
				}
				if (o.Kind == "context" || o.Context != nil) && !o.ObservedAt.Before(next.ContextAt[sk]) {
					s.Context = clone(o.Context)
					next.ContextAt[sk] = o.ObservedAt
				}
				if o.Provider == data.Claude && o.Source == "claude-statusline" && o.AuthMode != "api" && o.Kind == "quota" {
					var latest time.Time
					for _, q := range s.LocalQuotas {
						if q.ObservedAt.After(latest) {
							latest = q.ObservedAt
						}
					}
					if !o.ObservedAt.Before(latest) {
						s.LocalQuotas = clone(o.Quotas)
					}
				}
				next.Sessions[sk] = s
			}
		}
		if o.Tokens != nil {
			dk := key(o.Provider, o.AccountKey, o.RequestID)
			if _, duplicate := next.Dedup[dk]; !duplicate {
				if len(next.Dedup) >= maxDedup {
					return m.limit(ctx, "deduplication limit")
				}
				n, _ := tokenTotal(*o.Tokens, o.Provider)
				if o.Provider == data.Claude && o.AccountKey == "" && o.Source == "claude-otel" && o.AuthMode != "api" && o.ObservedAt.In(budapest).Format("2006-01-02") >= dailyCutoff(now) {
					if next.LocalDaily[o.Provider] == nil {
						next.LocalDaily[o.Provider] = map[string]int64{}
					}
					date := o.ObservedAt.In(budapest).Format("2006-01-02")
					prev := next.LocalDaily[o.Provider][date]
					if math.MaxInt64-prev < n {
						return fmt.Errorf("%w: local token overflow", module.ErrInvalid)
					}
					next.LocalDaily[o.Provider][date] = prev + n
				}
				if o.SessionID != "" {
					sk := key(o.Provider, o.AccountKey, o.SessionID)
					s := next.Sessions[sk]
					var prev int64
					if s.ConsumedTokens != nil {
						prev = *s.ConsumedTokens
					}
					if math.MaxInt64-prev < n {
						return fmt.Errorf("%w: token overflow", module.ErrInvalid)
					}
					sum := prev + n
					s.ConsumedTokens = &sum
					next.Sessions[sk] = s
				}
				if known && record.Provider.AuthMode == "subscription" && o.ObservedAt.In(budapest).Format("2006-01-02") >= dailyCutoff(now) {
					date := o.ObservedAt.In(budapest).Format("2006-01-02")
					if next.Daily[accountKey] == nil {
						next.Daily[accountKey] = map[string]int64{}
					}
					prev := next.Daily[accountKey][date]
					if math.MaxInt64-prev < n {
						return fmt.Errorf("%w: token overflow", module.ErrInvalid)
					}
					next.Daily[accountKey][date] = prev + n
				}
				next.Dedup[dk] = o.ObservedAt
			}
		}
	}
	next.UpdatedAt = now.UTC()
	if err := m.persist(ctx, next); err != nil {
		if err == errStorageLimit {
			return m.limit(ctx, "storage limit")
		}
		return err
	}
	m.state = next
	return nil
}

var errStorageLimit = fmt.Errorf("storage limit")

func (m *Module) limit(ctx context.Context, why string) error {
	next := clone(m.state)
	next.Partial = true
	if err := m.persist(ctx, next); err != nil {
		return err
	}
	m.state = next
	return fmt.Errorf("%w: %s; coverage partial", module.ErrUnavailable, why)
}
func (m *Module) persist(ctx context.Context, s diskState) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if len(b) > maxStorageBytes {
		return errStorageLimit
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.dir == "" {
		return nil
	}
	f, err := os.CreateTemp(m.dir, ".state-*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return replaceFile(ctx, name, filepath.Join(m.dir, "state-v1.json"))
}

var budapest = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Budapest")
	if err != nil {
		panic(err)
	}
	return loc
}()

func weekDate(now time.Time) string {
	local := now.In(budapest)
	days := (int(local.Weekday()) + 6) % 7
	return time.Date(local.Year(), local.Month(), local.Day()-days, 0, 0, 0, 0, budapest).Format("2006-01-02")
}
func prune(s *diskState, now time.Time) {
	for _, days := range s.LocalDaily {
		for date := range days {
			if date < dailyCutoff(now) {
				delete(days, date)
			}
		}
	}
	cutoff := now.AddDate(0, 0, -historyDays)
	day := dailyCutoff(now)
	for k, at := range s.Dedup {
		if at.Before(cutoff) {
			delete(s.Dedup, k)
		}
	}
	for k, v := range s.Sessions {
		if v.ObservedAt.Before(cutoff) {
			delete(s.Sessions, k)
			delete(s.ContextAt, k)
		}
	}
	for k, days := range s.Daily {
		for date := range days {
			if date < day {
				delete(days, date)
			}
		}
		if len(days) == 0 {
			delete(s.Daily, k)
		}
	}
	for k, a := range s.Accounts {
		for date := range a.Daily {
			if date < day {
				delete(a.Daily, date)
			}
		}
		if a.Provider.UpdatedAt.Before(cutoff) && s.Linked[a.Provider.ID] != a.Provider.AccountKey {
			delete(s.Accounts, k)
		} else {
			s.Accounts[k] = a
		}
	}
}

// Display-record reclamation never touches consumption aggregates or tombstones.
func reclaimSession(s *diskState, now time.Time) bool {
	victim := ""
	var oldest time.Time
	for k, v := range s.Sessions {
		threshold := 180 * time.Second
		if v.Source == "browser" {
			threshold = 45 * time.Second
		}
		if v.Activity != "completed" && now.Sub(v.ObservedAt) <= threshold {
			continue
		}
		if victim == "" || v.ObservedAt.Before(oldest) || v.ObservedAt.Equal(oldest) && k < victim {
			victim = k
			oldest = v.ObservedAt
		}
	}
	if victim == "" {
		return false
	}
	delete(s.Sessions, victim)
	delete(s.ContextAt, victim)
	return true
}
func dailyCutoff(now time.Time) string {
	return now.In(budapest).AddDate(0, 0, -historyDays+1).Format("2006-01-02")
}
func sumWeek(days map[string]int64, now time.Time) *int64 {
	start := weekDate(now)
	end := now.In(budapest).Format("2006-01-02")
	var total int64
	found := false
	for date, n := range days {
		if date >= start && date <= end {
			if math.MaxInt64-total < n {
				return nil
			}
			total += n
			found = true
		}
	}
	if !found {
		return nil
	}
	return &total
}
func (m *Module) Snapshot() data.State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	now := m.now()
	s := data.State{Providers: []data.Provider{}, Sessions: []data.Session{}, UpdatedAt: m.state.UpdatedAt, Partial: m.state.Partial}
	for _, id := range []string{data.OpenAI, data.Claude} {
		p := data.Provider{ID: id, Connection: "disconnected", AuthMode: "unknown", Quotas: []data.Quota{}, Coverage: "Unavailable", UsageLabel: "Tokens this week", Partial: m.state.Partial}
		ak, linked := m.state.Linked[id]
		if linked {
			a := m.state.Accounts[key(id, ak)]
			p = clone(a.Provider)
			p.Partial = m.state.Partial
			p.Stale = p.Connection != "connected" || now.Sub(p.UpdatedAt) > 180*time.Second
			p.Coverage = "Observed local; partial"
			p.UsageLabel = "Tokens this week"
			if p.AuthMode == "subscription" {
				if !a.AuthorityAt.IsZero() && len(a.Daily) > 0 {
					p.WeeklyTokens = sumWeek(a.Daily, now)
					p.UsageLabel = "Provider-day tokens"
					p.Coverage = "Provider account"
					if !a.Complete {
						p.Coverage += "; partial"
					}
					if now.Sub(a.AuthorityAt) > 180*time.Second {
						p.Stale = true
					}
				} else {
					p.WeeklyTokens = sumWeek(m.state.Daily[key(id, ak)], now)
				}
			}
			if p.WeeklyTokens == nil {
				p.Coverage = "Unavailable"
			}
			for _, q := range p.Quotas {
				freshness := 180 * time.Second
				if id == data.Claude {
					freshness = 5 * time.Minute
				}
				if now.Sub(q.ObservedAt) > freshness || q.ResetsAt != nil && !q.ResetsAt.After(now) {
					p.Stale = true
				}
			}
		}
		s.Providers = append(s.Providers, p)
		if id == data.Claude {
			s.Providers[len(s.Providers)-1].LocalWeeklyTokens = sumWeek(m.state.LocalDaily[id], now)
		}
	}
	for sessionKey, v := range m.state.Sessions {
		if v.AccountKey != "" && m.state.Linked[v.Provider] != v.AccountKey {
			continue
		}
		if now.Sub(v.ObservedAt) > historyDays*24*time.Hour {
			continue
		}
		v = clone(v)
		freshQuotas := v.LocalQuotas[:0]
		for _, q := range v.LocalQuotas {
			if now.Sub(q.ObservedAt) <= 180*time.Second && (q.ResetsAt == nil || q.ResetsAt.After(now)) {
				freshQuotas = append(freshQuotas, q)
			}
		}
		v.LocalQuotas = freshQuotas
		threshold := 180 * time.Second
		if v.Source == "browser" {
			threshold = 45 * time.Second
		}
		v.Stale = now.Sub(v.ObservedAt) > threshold
		if v.Stale {
			v.Activity = "unknown"
		}
		if at := m.state.ContextAt[sessionKey]; at.IsZero() || now.Sub(at) > threshold {
			v.Context = nil
		}
		s.Sessions = append(s.Sessions, v)
		for i := range s.Providers {
			p := &s.Providers[i]
			if p.ID == v.Provider {
				if len(v.LocalQuotas) > 0 && (len(p.LocalQuotas) == 0 || v.LocalQuotas[0].ObservedAt.After(p.LocalQuotas[0].ObservedAt)) {
					p.LocalQuotas = clone(v.LocalQuotas)
				}
				if v.Source == "browser" {
					p.Capabilities.BrowserActivity = true
				} else {
					p.Capabilities.LocalSessions = true
				}
				if v.Context != nil {
					p.Capabilities.ExactContext = true
				}
			}
		}
	}
	sort.Slice(s.Sessions, func(i, j int) bool {
		a, b := s.Sessions[i], s.Sessions[j]
		if !a.ObservedAt.Equal(b.ObservedAt) {
			return a.ObservedAt.After(b.ObservedAt)
		}
		return key(a.Provider, a.AccountKey, a.ID) < key(b.Provider, b.AccountKey, b.ID)
	})
	return s
}
func (m *Module) Render(ctx context.Context, view string) (image.Image, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	w, h := 640, 480
	if member(view, "openai", "claude") {
		h = 180
	} else if !member(view, "overview", "sessions") {
		return nil, module.ErrNotFound
	}
	frame := DrawWidget(m.Snapshot(), view, w, h, m.now(), true)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return frame, nil
}
