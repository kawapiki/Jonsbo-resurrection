package aiadapters

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

func TestRPCMatchesResponsesAndIgnoresNotifications(t *testing.T) {
	clientIn, serverOut := io.Pipe()
	serverIn, clientOut := io.Pipe()
	rpc := newRPC(clientIn, clientOut, func() { clientIn.Close(); clientOut.Close() }, nil)
	defer rpc.Close()
	go func() {
		line, _ := bufio.NewReader(serverIn).ReadBytes('\n')
		var req map[string]any
		json.Unmarshal(line, &req)
		json.NewEncoder(serverOut).Encode(map[string]any{"method": "unrelated", "params": map[string]any{"secret": "ignored"}})
		json.NewEncoder(serverOut).Encode(map[string]any{"id": req["id"], "result": map[string]any{"ok": true}})
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	raw, err := rpc.Call(ctx, "account/read", map[string]any{})
	if err != nil || !strings.Contains(string(raw), `"ok":true`) {
		t.Fatalf("%s %v", raw, err)
	}
}
func TestRPCCancellationAndMalformedProtocolReleaseCalls(t *testing.T) {
	clientIn, serverOut := io.Pipe()
	serverIn, clientOut := io.Pipe()
	closed := make(chan struct{})
	rpc := newRPC(clientIn, clientOut, func() { clientIn.Close(); clientOut.Close(); close(closed) }, nil)
	go func() { bufio.NewReader(serverIn).ReadBytes('\n'); serverOut.Write([]byte("invalid\n")) }()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := rpc.Call(ctx, "account/read", nil); err == nil {
		t.Fatal("bad protocol accepted")
	}
	rpc.Close()
	select {
	case <-closed:
	case <-ctx.Done():
		t.Fatal("close failed")
	}
}
func TestRPCCancelClosesBlockedWriter(t *testing.T) {
	in, out := io.Pipe()
	wi, wo := io.Pipe()
	defer out.Close()
	defer wi.Close()
	r := newRPC(in, wo, func() { in.Close(); wo.Close() }, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := r.Call(ctx, "blocked", nil); err == nil {
		t.Fatal("expected timeout")
	}
	if time.Since(start) > time.Second {
		t.Fatal("blocked writer leaked")
	}
	r.Close()
}
