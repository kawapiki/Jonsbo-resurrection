package aiadapters

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	data "github.com/kawapiki/Jonsbo-resurrection/pkg/aisubscriptions"
)

func nativeCodexHome() (string, error) {
	if home := strings.TrimSpace(os.Getenv("CODEX_HOME")); home != "" {
		return filepath.Abs(home)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errors.New("Native Codex home is unavailable")
	}
	return filepath.Join(home, ".codex"), nil
}

func (m *Manager) actionStatus(p string) ProviderStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.providers[p]
	s.Monitoring = m.enabled[p]
	return s
}

// check observes native installation/authentication only. It deliberately does
// not publish an account, fetch usage, or change saved monitoring preferences.
func (m *Manager) check(p string) string {
	m.op.Lock()
	defer m.op.Unlock()
	return m.checkLocked(p)
}

func (m *Manager) checkLocked(p string) string {
	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()
	if closed || m.ctx.Err() != nil {
		return "error"
	}
	if p == data.OpenAI {
		// A fresh native worker reads external CLI sign-in/logout and home changes;
		// an idle app-server may retain its previous authentication in memory.
		if m.rpc != nil {
			m.rpc.Close()
			m.rpc = nil
		}
		defer func() {
			m.mu.Lock()
			enabled := m.enabled[p]
			m.mu.Unlock()
			if !enabled && m.rpc != nil {
				m.rpc.Close()
				m.rpc = nil
			}
		}()
	}
	name := "codex"
	if p == data.Claude {
		name = "claude"
	}
	path, err := exec.LookPath(name)
	if err == nil {
		switch strings.ToLower(filepath.Ext(path)) {
		case ".cmd", ".bat", ".ps1":
			err = errors.New("native executable required")
		}
	}
	m.mu.Lock()
	s := m.providers[p]
	s.Ready = err == nil
	s.Login = "checking"
	m.providers[p] = s
	m.mu.Unlock()
	if err != nil {
		m.loginState(p, "unavailable")
		return "unavailable"
	}
	m.paths[p] = path
	ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
	defer cancel()
	_, login, _ := m.nativeAccount(ctx, p, false)
	m.loginState(p, login)
	return login
}

func (m *Manager) loginState(p, login string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.providers[p]
	s.Login = login
	s.Monitoring = m.enabled[p]
	if !s.Monitoring {
		s.Connection = "disconnected"
	}
	switch login {
	case "signed-in":
		s.Message = "Native subscription is signed in; start monitoring"
		if s.Monitoring {
			s.Message = "Monitoring native subscription sign-in"
			s.Connection = "connected"
		}
	case "signed-out":
		s.Message = "Sign in to the native subscription account"
		if s.Monitoring {
			s.Connection = "reauthentication"
		}
	case "api-key":
		s.Message = "Native client uses API authentication; subscription sign-in is required"
		if s.Monitoring {
			s.Connection = "reauthentication"
		}
	case "unavailable":
		s.Connection = "unavailable"
		s.Message = "Install the native CLI executable, then check again; shell wrappers are unsupported"
	default:
		s.Message = "Native authentication could not be checked; check the installation and retry"
		if s.Monitoring {
			s.Connection = "reauthentication"
		}
	}
	m.providers[p] = s
}

// nativeAccount never starts a login or model turn. Detection uses the native
// account/read without token refresh, and does not retain identity in status.
func (m *Manager) nativeAccount(ctx context.Context, p string, refresh bool) (data.Observation, string, error) {
	var raw []byte
	var err error
	if p == data.Claude {
		raw, err = m.runner.Run(ctx, m.paths[p], []string{"auth", "status", "--json"})
	} else {
		err = m.ensureCodex(ctx)
		if err == nil {
			raw, err = m.rpc.Call(ctx, "account/read", map[string]any{"refreshToken": refresh})
		}
	}
	if ctx.Err() != nil {
		return data.Observation{}, "error", errors.New("Native authentication check canceled")
	}
	object, parseErr := object(raw)
	if parseErr != nil {
		return data.Observation{}, "error", errors.New("Native authentication could not be checked")
	}
	if p == data.Claude {
		logged, present := object["loggedIn"].(bool)
		if !present {
			return data.Observation{}, "error", errors.New("Native authentication status is unsupported")
		}
		if !logged {
			return data.Observation{}, "signed-out", errors.New("Native subscription sign-in required")
		}
		if str(object["authMethod"]) != "claude.ai" {
			return data.Observation{}, "api-key", errors.New("Native subscription authentication required")
		}
	} else {
		a := obj(object["account"])
		if object["account"] == nil {
			return data.Observation{}, "signed-out", errors.New("Native subscription sign-in required")
		}
		if str(a["type"]) == "apiKey" {
			return data.Observation{}, "api-key", errors.New("Native subscription authentication required")
		}
		if str(a["type"]) != "chatgpt" {
			return data.Observation{}, "error", errors.New("Native authentication status is unsupported")
		}
	}
	if err != nil {
		return data.Observation{}, "error", errors.New("Native authentication could not be checked")
	}
	var account data.Observation
	if p == data.Claude {
		account, err = parseClaudeAccount(raw, time.Now().UTC())
	} else {
		account, err = parseOpenAIAccount(raw, time.Now().UTC())
	}
	if err != nil {
		return account, "error", errors.New("Native account identity is unavailable; update the CLI")
	}
	return account, "signed-in", nil
}

func (m *Manager) enableLocked(p string) error {
	m.mu.Lock()
	previous := m.enabled[p]
	m.enabled[p] = true
	m.mu.Unlock()
	if err := m.save(); err != nil {
		m.mu.Lock()
		m.enabled[p] = previous
		m.mu.Unlock()
		return errors.New("Monitoring preferences could not be saved")
	}
	m.loginState(p, "signed-in")
	m.signal()
	return nil
}

func (m *Manager) monitor(p string) error {
	m.op.Lock()
	defer m.op.Unlock()
	if m.checkLocked(p) != "signed-in" {
		return errors.New("An existing native subscription sign-in is required to start monitoring")
	}
	return m.enableLocked(p)
}
