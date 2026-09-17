// The event producer demonstrates the external notification/agent boundary.
// It sends data to an already running local controller; it never executes events.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("event-producer", flag.ContinueOnError)
	flags.SetOutput(out)
	base := flags.String("url", "http://127.0.0.1:8787", "local controller URL")
	path := flags.String("token-file", "", "controller bearer token file (or JONSBO_API_TOKEN environment variable)")
	message := flags.String("message", "", "message to publish (1–200 characters)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	token := strings.TrimSpace(os.Getenv("JONSBO_API_TOKEN"))
	if *path != "" {
		f, err := os.Open(*path)
		if err != nil {
			return errors.New("cannot open token file")
		}
		defer f.Close()
		data, err := io.ReadAll(io.LimitReader(f, 4097))
		if err != nil || len(data) > 4096 {
			return errors.New("cannot read bounded token file")
		}
		token = strings.TrimSpace(string(data))
	}
	if err := send(ctx, *base, token, *message); err != nil {
		return err
	}
	_, err := fmt.Fprintln(out, "Message accepted.")
	return err
}

func send(ctx context.Context, base, token, message string) error {
	if token == "" || len(token) > 4096 || strings.ContainsAny(token, "\r\n\t ") {
		return errors.New("provide a valid token file or JONSBO_API_TOKEN")
	}
	if !utf8.ValidString(message) || strings.TrimSpace(message) == "" || utf8.RuneCountInString(message) > 200 {
		return errors.New("message requires 1–200 UTF-8 characters")
	}
	u, err := url.Parse(base)
	if err != nil {
		return errors.New("invalid local controller URL")
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "http" || ip == nil || !ip.IsLoopback() || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return errors.New("URL must be an HTTP loopback IP address without credentials, path, query or fragment")
	}
	u.Path = "/v1/modules/example/events"
	payload := struct {
		Type    string `json:"type"`
		Payload struct {
			Text string `json:"text"`
		} `json:"payload"`
	}{Type: "message"}
	payload.Payload.Text = message
	data, err := json.Marshal(payload)
	if err != nil {
		return errors.New("cannot encode event")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(data))
	if err != nil {
		return errors.New("cannot create event request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	// No proxy or redirect can receive the local bearer credential.
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 3 * time.Second}).DialContext}
	defer transport.CloseIdleConnections()
	client := &http.Client{Timeout: 5 * time.Second, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("local event request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("controller rejected event (HTTP %d)", resp.StatusCode)
	}
	return nil
}
