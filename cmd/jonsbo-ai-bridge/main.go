// jonsbo-ai-bridge accepts Chromium native messages and forwards visible metadata.
package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/kawapiki/Jonsbo-resurrection/internal/api"
)

const maxMessage = 64 << 10

type Config struct {
	API         string `json:"api"`
	TokenFile   string `json:"token_file"`
	ExtensionID string `json:"extension_id"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Jonsbo browser bridge: configuration or native message unavailable")
		os.Exit(1)
	}
}
func run() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	b, err := readBounded(filepath.Join(filepath.Dir(exe), "jonsbo-ai-bridge.json"), 4096)
	if err != nil {
		return err
	}
	var c Config
	if err = json.Unmarshal(b, &c); err != nil {
		return err
	}
	if err = validateAPI(c.API); err != nil {
		return err
	}
	if len(c.ExtensionID) != 32 || strings.Trim(c.ExtensionID, "abcdefghijklmnop") != "" {
		return errors.New("extension ID required")
	}
	if len(os.Args) < 2 || os.Args[1] != "chrome-extension://"+c.ExtensionID+"/" {
		return errors.New("paired extension required")
	}
	b, err = readBounded(c.TokenFile, 256)
	if err != nil {
		return err
	}
	token := strings.TrimSpace(string(b))
	decoded, err := hex.DecodeString(token)
	if err != nil || len(decoded) != 32 {
		return errors.New("invalid scoped credential")
	}
	return serve(context.Background(), os.Stdin, os.Stdout, c, token)
}
func readBounded(path string, max int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, max+1))
	if int64(len(b)) > max {
		return nil, errors.New("file exceeds limit")
	}
	return b, err
}
func validateAPI(s string) error {
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return errors.New("numeric loopback HTTP required")
	}
	return api.ValidateListen(u.Host)
}

type visible struct {
	URL       string `json:"url"`
	Provider  string `json:"provider"`
	SessionID string `json:"session_id"`
	Title     string `json:"title,omitempty"`
	Model     string `json:"model,omitempty"`
	Activity  string `json:"activity"`
}

func clean(s string, n int) string {
	r := []rune(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s))
	if len(r) > n {
		r = r[:n]
	}
	return string(r)
}
func project(raw []byte) ([]byte, error) {
	var v visible
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	u, err := url.Parse(v.URL)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return nil, errors.New("unsupported tab URL")
	}
	path := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(path) < 2 || len(v.SessionID) > 256 || v.SessionID == "" {
		return nil, errors.New("conversation required")
	}
	valid := v.Provider == "openai" && u.Host == "chatgpt.com" && path[0] == "c" || v.Provider == "claude" && u.Host == "claude.ai" && path[0] == "chat"
	if !valid {
		return nil, errors.New("unsupported provider tab")
	}
	if v.Activity != "running" && v.Activity != "idle" && v.Activity != "waiting" && v.Activity != "completed" && v.Activity != "unknown" {
		v.Activity = "unknown"
	}
	// Only these fields leave the host; input account/tokens/content claims disappear.
	return json.Marshal(struct {
		Provider  string `json:"provider"`
		SessionID string `json:"session_id"`
		Title     string `json:"title,omitempty"`
		Model     string `json:"model,omitempty"`
		Activity  string `json:"activity"`
	}{v.Provider, clean(v.SessionID, 256), clean(v.Title, 128), clean(v.Model, 80), v.Activity})
}
func serve(ctx context.Context, input io.Reader, output io.Writer, c Config, token string) error {
	client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true, DialContext: (&net.Dialer{Timeout: time.Second}).DialContext}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	for {
		var header [4]byte
		if _, err := io.ReadFull(input, header[:]); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		n := binary.LittleEndian.Uint32(header[:])
		if n == 0 || n > maxMessage {
			return errors.New("native message exceeds limit")
		}
		raw := make([]byte, n)
		if _, err := io.ReadFull(input, raw); err != nil {
			return err
		}
		metadata, err := project(raw)
		accepted := false
		if err == nil {
			requestCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			r, e := http.NewRequestWithContext(requestCtx, "POST", strings.TrimRight(c.API, "/")+"/v1/ai/browser/events", bytes.NewReader(metadata))
			if e == nil {
				r.Header.Set("Authorization", "Bearer "+token)
				r.Header.Set("Content-Type", "application/json")
				response, e := client.Do(r)
				if e == nil {
					accepted = response.StatusCode == 202
					io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
					response.Body.Close()
				}
			}
			cancel()
		}
		reply, _ := json.Marshal(map[string]bool{"accepted": accepted})
		binary.LittleEndian.PutUint32(header[:], uint32(len(reply)))
		if _, err := output.Write(header[:]); err != nil {
			return err
		}
		if _, err := output.Write(reply); err != nil {
			return err
		}
	}
}
