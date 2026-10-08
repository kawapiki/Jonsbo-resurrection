// jonsbo-ai-observer is an optional metadata-only native hook/status-line helper.
package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"github.com/kawapiki/Jonsbo-resurrection/internal/aiadapters"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "Jonsbo observer: invalid arguments or metadata; check setup instructions")
		os.Exit(2)
	}
}
func run(args []string, input io.Reader, output io.Writer) error {
	if len(args) == 0 || (args[0] != "hook" && args[0] != "statusline") {
		return errors.New("expected hook or statusline")
	}
	mode := args[0]
	f := flag.NewFlagSet("observer", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	provider := f.String("provider", "", "provider")
	api := f.String("api", "http://127.0.0.1:8080", "loopback API")
	tokenFile := f.String("token-file", "", "scoped credential file")
	if err := f.Parse(args[1:]); err != nil {
		return err
	}
	if f.NArg() != 0 || (*provider != "openai" && *provider != "claude") || *tokenFile == "" {
		return errors.New("invalid observer arguments")
	}
	if mode == "statusline" && *provider != "claude" {
		return errors.New("statusline requires Claude")
	}
	u, err := url.Parse(*api)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return errors.New("loopback HTTP API required")
	}
	ip := net.ParseIP(u.Hostname())
	if strings.EqualFold(u.Hostname(), "localhost") {
		port := u.Port()
		if port == "" {
			port = "80"
		}
		u.Host = net.JoinHostPort("127.0.0.1", port)
		*api = u.String()
		ip = net.ParseIP("127.0.0.1")
	}
	if ip == nil || !ip.IsLoopback() {
		return errors.New("numeric loopback destination required")
	}
	if mode == "hook" && *provider == "openai" {
		defer fmt.Fprintln(output, "{}")
	}
	raw, err := io.ReadAll(io.LimitReader(input, aiadapters.MaxPayload+1))
	if err != nil {
		return nil
	}
	metadata, err := aiadapters.NativeMetadata(*provider, mode, raw)
	if err != nil {
		return nil
	}
	if mode == "statusline" {
		obs, err := aiadapters.ProjectNative(*provider, mode, metadata, time.Now())
		if err == nil && len(obs) > 0 {
			model := obs[0].Model
			if model == "" {
				model = "Claude"
			}
			pct := "--"
			if obs[0].Context != nil && obs[0].Context.UsedPercent != nil {
				pct = fmt.Sprintf("%.0f%%", *obs[0].Context.UsedPercent)
			}
			fmt.Fprintf(output, "[%s] %s context\n", model, pct)
		}
	}
	tokenRaw, err := os.ReadFile(*tokenFile)
	if err != nil || len(tokenRaw) > 256 {
		return nil
	}
	token := strings.TrimSpace(string(tokenRaw))
	decoded, err := hex.DecodeString(token)
	if err != nil || len(decoded) != 32 {
		return nil
	}
	endpoint := strings.TrimRight(*api, "/") + "/v1/ai/observer/" + *provider + "/" + mode
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	r, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(metadata))
	if err != nil {
		return nil
	}
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/json")
	// A scoped credential must never follow a redirect or inherited proxy.
	client := &http.Client{Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}}
	response, err := client.Do(r)
	if err != nil {
		return nil
	}
	defer response.Body.Close()
	io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	return nil
}
