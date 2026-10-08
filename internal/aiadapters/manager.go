package aiadapters

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	data "github.com/kawapiki/Jonsbo-resurrection/pkg/aisubscriptions"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Sink func(context.Context, data.Observation) error
type ProviderStatus struct {
	ID         string `json:"id"`
	Ready      bool   `json:"ready"`
	Login      string `json:"login"`
	Monitoring bool   `json:"monitoring"`
	Connection string `json:"connection"`
	Message    string `json:"message"`
}
type Manager struct {
	dir                         string
	sink                        Sink
	browserToken, observerToken string
	mu                          sync.Mutex
	op                          sync.Mutex
	providers                   map[string]ProviderStatus
	paths                       map[string]string
	enabled                     map[string]bool
	accountAliases              map[string]string // verified native identity -> backend identity; guarded by op
	runner                      commandRunner
	rpc                         *rpcClient
	ctx                         context.Context
	cancel                      context.CancelFunc
	nextClaudeUsage             time.Time // guarded by op; native usage is polled at most every three minutes
	claudeUsageAccount          string
	claudeUsageError            error
	claudeUsageTimeout          time.Duration
	wake                        chan struct{}
	running, closed             bool
}

func New(dir string, sink Sink) (*Manager, error) {
	if sink == nil {
		return nil, errors.New("observation sink is required")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	m := &Manager{dir: dir, sink: sink, runner: nativeRunner{}, providers: map[string]ProviderStatus{}, paths: map[string]string{}, enabled: map[string]bool{}, accountAliases: map[string]string{}, wake: make(chan struct{}, 1)}
	m.ctx, m.cancel = context.WithCancel(context.Background())
	var err error
	m.browserToken, err = credential(filepath.Join(dir, "browser.token"))
	if err != nil {
		return nil, err
	}
	m.observerToken, err = credential(filepath.Join(dir, "observer.token"))
	if err != nil {
		return nil, err
	}
	for _, p := range []string{data.OpenAI, data.Claude} {
		name := "codex"
		if p == data.Claude {
			name = "claude"
		}
		path, e := exec.LookPath(name)
		if e == nil {
			ext := strings.ToLower(filepath.Ext(path))
			if ext == ".cmd" || ext == ".bat" || ext == ".ps1" {
				e = errors.New("native executable required")
			}
		}
		m.paths[p] = path
		s := ProviderStatus{ID: p, Ready: e == nil, Login: "checking", Connection: "disconnected", Message: "Checking native subscription sign-in"}
		if e != nil {
			s.Login = "unavailable"
			s.Connection = "unavailable"
			s.Message = "Install the native " + name + " executable; shell wrappers are unsupported"
		}
		m.providers[p] = s
	}
	if raw, err := os.ReadFile(filepath.Join(dir, "connections.json")); err == nil && len(raw) < 4096 {
		var saved map[string]bool
		if json.Unmarshal(raw, &saved) == nil {
			for _, p := range []string{data.OpenAI, data.Claude} {
				m.enabled[p] = saved[p]
			}
		}
	}
	for _, p := range []string{data.OpenAI, data.Claude} {
		s := m.providers[p]
		s.Monitoring = m.enabled[p]
		m.providers[p] = s
	}
	if file, err := os.Open(filepath.Join(dir, "account-aliases.json")); err == nil {
		raw, readErr := io.ReadAll(io.LimitReader(file, (32<<10)+1))
		file.Close()
		var aliases map[string]string
		if readErr == nil && len(raw) <= 32<<10 && json.Unmarshal(raw, &aliases) == nil && len(aliases) <= 128 {
			for key, value := range aliases {
				keyBytes, keyErr := hex.DecodeString(key)
				valueBytes, valueErr := hex.DecodeString(value)
				if keyErr == nil && valueErr == nil && len(keyBytes) == 16 && len(valueBytes) == 16 {
					m.accountAliases[key] = value
				}
			}
		}
	}
	return m, nil
}

func (m *Manager) saveAccountAliases() error {
	raw, err := json.Marshal(m.accountAliases)
	if err != nil {
		return err
	}
	tmp := filepath.Join(m.dir, "account-aliases.json.tmp")
	if err = os.WriteFile(tmp, raw, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(m.dir, "account-aliases.json"))
}
func credential(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err == nil {
		token := strings.TrimSpace(string(raw))
		decoded, e := hex.DecodeString(token)
		if e != nil || len(decoded) != 32 {
			return "", errors.New("invalid scoped credential file")
		}
		return token, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	var secret [32]byte
	if _, err = rand.Read(secret[:]); err != nil {
		return "", err
	}
	token := hex.EncodeToString(secret[:])
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	_, err = f.WriteString(token + "\n")
	closeErr := f.Close()
	if err != nil {
		return "", err
	}
	return token, closeErr
}
func (m *Manager) signal() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}
func (m *Manager) set(p, connection, message string) {
	m.mu.Lock()
	s := m.providers[p]
	s.Connection = connection
	s.Message = message
	m.providers[p] = s
	m.mu.Unlock()
}
func (m *Manager) emit(ctx context.Context, o data.Observation) error {
	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()
	if closed {
		return errors.New("adapter stopped")
	}
	return m.sink(ctx, o)
}
func (m *Manager) save() error {
	m.mu.Lock()
	raw, err := json.Marshal(m.enabled)
	m.mu.Unlock()
	if err != nil {
		return err
	}
	tmp := filepath.Join(m.dir, "connections.json.tmp")
	if err = os.WriteFile(tmp, raw, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(m.dir, "connections.json"))
}

// Run owns native process lifetime. It starts no login and sends no model turns.
func (m *Manager) Run(ctx context.Context) error {
	m.mu.Lock()
	if m.running || m.closed {
		m.mu.Unlock()
		return errors.New("adapter already running or closed")
	}
	m.running = true
	m.mu.Unlock()
	// Refresh and detection workers derive from m.ctx. Propagate parent shutdown
	// immediately, including while Run is blocked inside a native account call.
	stopParentCancellation := context.AfterFunc(ctx, m.cancel)
	defer stopParentCancellation()
	defer func() {
		m.cancel()
		m.op.Lock()
		if m.rpc != nil {
			m.rpc.Close()
			m.rpc = nil
		}
		m.mu.Lock()
		m.running = false
		m.closed = true
		m.mu.Unlock()
		m.op.Unlock()
	}()
	timer := time.NewTimer(0)
	defer timer.Stop()
	failures := 0
	for {
		select {
		case <-ctx.Done():
			m.cancel()
			return nil
		case <-m.ctx.Done():
			return nil
		case <-timer.C:
		case <-m.wake:
		}
		m.mu.Lock()
		enabled := map[string]bool{}
		for p, v := range m.enabled {
			enabled[p] = v
		}
		m.mu.Unlock()
		failed := false
		for _, p := range []string{data.OpenAI, data.Claude} {
			if enabled[p] {
				if m.refresh(p) != nil {
					failed = true
				}
			} else {
				m.check(p)
			}
		}
		delay := 60 * time.Second
		if failed {
			failures++
			if failures > 4 {
				failures = 4
			}
			delay = time.Duration(60*(1<<uint(failures-1))) * time.Second
		} else {
			failures = 0
		}
		var jitter [1]byte
		rand.Read(jitter[:])
		delay += time.Duration(jitter[0]%6) * time.Second
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(delay)
	}
}

func (m *Manager) ensureCodex(ctx context.Context) error {
	if m.rpc != nil {
		select {
		case <-m.rpc.done:
			m.rpc = nil
		default:
			return nil
		}
	}
	home, err := nativeCodexHome()
	if err != nil {
		return err
	}
	rpc, err := m.runner.StartCodex(m.ctx, m.paths[data.OpenAI], home, func(method string, _ json.RawMessage) {
		if method == "account/login/completed" || method == "account/updated" {
			m.signal()
		}
	})
	if err != nil {
		return err
	}
	m.rpc = rpc
	if _, err = rpc.Call(ctx, "initialize", map[string]any{"clientInfo": map[string]string{"name": "jonsbo_subscription_observer", "version": "1.0"}}); err != nil {
		rpc.Close()
		m.rpc = nil
		return errors.New("Codex app-server initialization failed; update native Codex")
	}
	if err = rpc.Initialized(ctx); err != nil {
		rpc.Close()
		m.rpc = nil
	}
	return err
}

func (m *Manager) refresh(p string) error {
	m.op.Lock()
	defer m.op.Unlock()
	m.mu.Lock()
	enabled := m.enabled[p]
	ready := m.providers[p].Ready
	closed := m.closed
	m.mu.Unlock()
	if !enabled || !ready || closed {
		return errors.New("provider is detached or unavailable")
	}
	ctx, cancel := context.WithTimeout(m.ctx, 20*time.Second)
	defer cancel()
	// Native authentication can be cached at app-server startup. Reopen before
	// every enabled poll so external CLI logout, login and home changes are read.
	if p == data.OpenAI && m.rpc != nil {
		m.rpc.Close()
		m.rpc = nil
	}
	now := time.Now().UTC()
	account, login, err := m.nativeAccount(ctx, p, true)
	m.loginState(p, login)
	if err != nil {
		m.set(p, "reauthentication", err.Error())
		o := base(p, "claude-auth", "", now)
		if p == data.OpenAI {
			o.Source = "openai-app-server"
		}
		o.Kind = "account"
		o.Connection = "reauthentication"
		m.emit(m.ctx, o)
		return err
	}
	var quotas *data.Observation
	var projectionError error
	claudeIdentityFailure := false
	if p == data.Claude {
		if m.claudeUsageAccount != account.AccountKey {
			m.claudeUsageAccount = account.AccountKey
			m.nextClaudeUsage = time.Time{}
			m.claudeUsageError = nil
		}
		if !now.Before(m.nextClaudeUsage) {
			m.nextClaudeUsage = now.Add(3 * time.Minute)
			reader, supported := m.runner.(interface {
				ClaudeUsage(context.Context, string) ([]byte, error)
			})
			if !supported {
				projectionError = errors.New("native Claude background usage reader is unavailable")
			} else {
				usageTimeout := m.claudeUsageTimeout
				if usageTimeout <= 0 {
					usageTimeout = 12 * time.Second
				}
				usageCtx, usageCancel := context.WithTimeout(m.ctx, usageTimeout)
				raw, usageErr := reader.ClaudeUsage(usageCtx, m.paths[p])
				usageCancel()
				// Verify with a fresh deadline: a quota timeout does not imply logout.
				verifyCtx, verifyCancel := context.WithTimeout(m.ctx, 5*time.Second)
				verified, verifiedLogin, verifyErr := m.nativeAccount(verifyCtx, p, false)
				verifyCancel()
				if verifyErr != nil {
					m.loginState(p, verifiedLogin)
					projectionError = errors.New("native Claude account verification failed")
					account.Connection = "reauthentication"
					claudeIdentityFailure = true
				} else if verified.AccountKey != account.AccountKey || verified.Plan != account.Plan {
					account = verified
					projectionError = errors.New("Claude account changed during usage refresh; retrying")
					claudeIdentityFailure = true
				} else if usageErr != nil {
					projectionError = errors.New("native Claude quota refresh is unavailable")
					if usageErr.Error() == "background usage requires a newer Claude Code" {
						projectionError = errors.New("background usage requires a newer Claude Code")
					}
				} else if q, e := parseClaudeQuotas(raw, account.AccountKey, time.Now().UTC()); e != nil {
					projectionError = e
				} else {
					quotas = &q
				}
			}
			if claudeIdentityFailure {
				m.nextClaudeUsage = time.Time{}
			}
			m.claudeUsageError = projectionError
		} else {
			projectionError = m.claudeUsageError
		}
	}
	if p == data.OpenAI {
		verifiedIdentity := account.AccountKey
		if canonical := m.accountAliases[verifiedIdentity]; canonical != "" {
			account.AccountKey = canonical
		}
		if raw, e := m.rpc.Call(ctx, "account/rateLimits/read", map[string]any{}); e == nil {
			parsed, _ := object(raw)
			if id := str(parsed["accountId"]); id != "" {
				account.AccountKey = opaque(data.OpenAI, id)
				previous := m.accountAliases[verifiedIdentity]
				if previous != account.AccountKey {
					if previous == "" && len(m.accountAliases) >= 128 {
						return errors.New("native account identity cache is full; disconnect before linking another account")
					}
					m.accountAliases[verifiedIdentity] = account.AccountKey
					if err = m.saveAccountAliases(); err != nil {
						if previous == "" {
							delete(m.accountAliases, verifiedIdentity)
						} else {
							m.accountAliases[verifiedIdentity] = previous
						}
						return errors.New("native account identity cache could not be persisted")
					}
				}
			}
			// Timestamp the completed read. Authentication can take longer than
			// the intake's one-second ordering tolerance after a cold startup.
			q, e := parseOpenAIQuotas(raw, account.AccountKey, time.Now().UTC())
			if e == nil {
				quotas = &q
			} else {
				projectionError = errors.New("native quota metadata projection failed")
			}
		}
	}
	if err = m.emit(m.ctx, account); err != nil {
		return err
	}
	m.set(p, account.Connection, "Connected through native subscription authentication")
	if projectionError != nil {
		message := "Connected; " + projectionError.Error()
		if account.Connection != "connected" {
			message = projectionError.Error()
		}
		m.set(p, account.Connection, message)
		// Optional Claude quota retries must not slow healthy account polling.
		// The native quota reader has its own three-minute retry interval.
		if p == data.Claude && !claudeIdentityFailure {
			return nil
		}
		return projectionError
	}
	if quotas != nil && len(quotas.Quotas) > 0 {
		if err = m.emit(m.ctx, *quotas); err != nil {
			m.set(p, "connected", "Connected; native quota metadata intake rejected")
			return errors.New("native quota metadata intake rejected")
		}
	}
	if p == data.OpenAI {
		if raw, e := m.rpc.Call(ctx, "account/usage/read", map[string]any{}); e == nil {
			u, e := parseOpenAIUsage(raw, account.AccountKey, time.Now().UTC())
			if e != nil {
				m.set(p, "connected", "Connected; native usage metadata projection failed")
				return errors.New("native usage metadata projection failed")
			}
			if len(u.DailyUsage) > 0 {
				if err = m.emit(m.ctx, u); err != nil {
					m.set(p, "connected", "Connected; native usage metadata intake rejected")
					return errors.New("native usage metadata intake rejected")
				}
			}
		}
	}
	return nil
}

// connect is retained for older configurators. It only starts monitoring of an
// existing native subscription sign-in; users authenticate in the native CLI.
func (m *Manager) connect(p string) (map[string]string, error) {
	if err := m.monitor(p); err != nil {
		return nil, err
	}
	return map[string]string{"connection": "connected", "message": "Monitoring existing native subscription sign-in"}, nil
}
func (m *Manager) disconnect(p string) error {
	m.op.Lock()
	defer m.op.Unlock()
	m.mu.Lock()
	m.enabled[p] = false
	s := m.providers[p]
	s.Monitoring = false
	m.providers[p] = s
	m.mu.Unlock()
	if p == data.OpenAI && m.rpc != nil {
		m.rpc.Close()
		m.rpc = nil
	}
	if p == data.Claude {
		m.nextClaudeUsage = time.Time{}
		m.claudeUsageAccount = ""
		m.claudeUsageError = nil
	}
	m.set(p, "disconnected", "Monitoring stopped; native sign-in is retained")
	err := m.save()
	if p == data.OpenAI {
		m.accountAliases = map[string]string{}
		if e := m.saveAccountAliases(); err == nil {
			err = e
		}
	}
	o := base(p, "claude-auth", "", time.Now())
	if p == data.OpenAI {
		o.Source = "openai-app-server"
	}
	o.Kind = "disconnect"
	o.Connection = "disconnected"
	if e := m.emit(m.ctx, o); err == nil {
		err = e
	}
	return err
}

func (m *Manager) AuthenticateIngress(r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	path := r.URL.Path
	token := ""
	if path == "/v1/ai/browser/events" {
		token = m.browserToken
	} else {
		for _, p := range []string{data.OpenAI, data.Claude} {
			if path == "/v1/ai/observer/"+p+"/hook" || (p == data.Claude && path == "/v1/ai/observer/claude/statusline") || path == "/v1/ai/telemetry/"+p+"/v1/logs" {
				token = m.observerToken
			}
		}
	}
	if token == "" {
		return false
	}
	got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	return subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1
}
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func (m *Manager) Handler() http.Handler { return http.HandlerFunc(m.serve) }
func (m *Manager) serve(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()
	if closed {
		respond(w, 503, map[string]string{"error": "adapter stopped"})
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1/ai/")
	if strings.HasPrefix(path, "browser/") || strings.HasPrefix(path, "observer/") || strings.HasPrefix(path, "telemetry/") {
		// Ingress is never a browser CORS endpoint. Root also retains Host/Origin checks.
		if r.Header.Get("Origin") != "" {
			respond(w, 403, map[string]string{"error": "native ingress does not accept browser origins"})
			return
		}
		if !m.AuthenticateIngress(r) {
			respond(w, 401, map[string]string{"error": "scoped ingress credential required"})
			return
		}
		m.ingest(w, r, path)
		return
	}
	switch {
	case path == "status" && r.Method == "GET":
		m.mu.Lock()
		out := []ProviderStatus{m.providers[data.OpenAI], m.providers[data.Claude]}
		running := m.running
		m.mu.Unlock()
		respond(w, 200, map[string]any{"providers": out, "running": running})
	case path == "setup" && r.Method == "GET":
		respond(w, 200, m.setup(r))
	case strings.HasPrefix(path, "providers/") && r.Method == "POST":
		parts := strings.Split(path, "/")
		if len(parts) != 3 || (parts[1] != data.OpenAI && parts[1] != data.Claude) {
			respond(w, 404, map[string]string{"error": "unknown provider"})
			return
		}
		p := parts[1]
		var result any
		var err error
		switch parts[2] {
		case "check":
			m.check(p)
			result = m.actionStatus(p)
		case "monitor":
			err = m.monitor(p)
			result = m.actionStatus(p)
		case "connect":
			_, err = m.connect(p)
			result = m.actionStatus(p)
		case "disconnect":
			err = m.disconnect(p)
			result = m.actionStatus(p)
		case "refresh":
			err = m.refresh(p)
			result = m.actionStatus(p)
		default:
			respond(w, 404, map[string]string{"error": "unknown action"})
			return
		}
		if err != nil {
			respond(w, 503, map[string]string{"error": "Provider action failed; check native installation and subscription sign-in"})
			return
		}
		respond(w, 200, result)
	default:
		respond(w, 404, map[string]string{"error": "unknown AI route"})
	}
}
func (m *Manager) ingest(w http.ResponseWriter, r *http.Request, path string) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxPayload))
	if err != nil {
		respond(w, 413, map[string]string{"error": "payload exceeds 1 MiB"})
		return
	}
	var observations []data.Observation
	now := time.Now().UTC()
	if path == "browser/events" {
		observations, err = projectBrowser(raw, now)
	} else {
		parts := strings.Split(path, "/")
		if parts[0] == "observer" {
			observations, err = ProjectNative(parts[1], parts[2], raw, now)
		} else {
			observations, err = ProjectOTLP(parts[1], raw, now)
		}
	}
	if err != nil {
		respond(w, 400, map[string]string{"error": err.Error()})
		return
	}
	for _, o := range observations {
		if err = m.emit(r.Context(), o); err != nil {
			respond(w, 503, map[string]string{"error": "metadata intake unavailable"})
			return
		}
	}
	respond(w, 202, map[string]int{"accepted": len(observations)})
}
func projectBrowser(raw []byte, now time.Time) ([]data.Observation, error) {
	m, err := object(raw)
	if err != nil {
		return nil, err
	}
	events := []any{m}
	if a, ok := m["events"]; ok {
		events = list(a)
	}
	if len(events) == 0 || len(events) > 64 {
		return nil, errors.New("browser batch must contain 1 to 64 events")
	}
	var out []data.Observation
	for _, v := range events {
		e := obj(v)
		p := str(e["provider"])
		sid := str(e["session_id"])
		if (p != data.OpenAI && p != data.Claude) || sid == "" {
			return nil, errors.New("invalid browser provider or session")
		}
		o := base(p, "browser", sid, now)
		o.Kind = "session"
		o.Title = str(e["title"])
		o.Model = str(e["model"])
		o.Activity = str(e["activity"])
		switch o.Activity {
		case "generating":
			o.Activity = "running"
		case "running", "idle", "waiting", "completed", "unknown":
		default:
			o.Activity = "unknown"
		}
		out = append(out, o)
	}
	return out, nil
}
