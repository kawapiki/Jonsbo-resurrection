package aiadapters

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type rpcReply struct {
	ID     uint64          `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code int `json:"code"`
	} `json:"error"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}
type rpcClient struct {
	input   io.ReadCloser
	output  io.WriteCloser
	stop    func()
	once    sync.Once
	calls   sync.Mutex
	next    uint64
	replies chan rpcReply
	done    chan struct{}
	notify  func(string, json.RawMessage)
}

func newRPC(input io.ReadCloser, output io.WriteCloser, stop func(), notify func(string, json.RawMessage)) *rpcClient {
	r := &rpcClient{input: input, output: output, stop: stop, replies: make(chan rpcReply, 8), done: make(chan struct{}), notify: notify}
	go func() {
		defer r.Close()
		s := bufio.NewScanner(input)
		s.Buffer(make([]byte, 4096), MaxPayload)
		for s.Scan() {
			var reply rpcReply
			if json.Unmarshal(s.Bytes(), &reply) != nil {
				return
			}
			if reply.Method != "" {
				if r.notify != nil {
					r.notify(reply.Method, reply.Params)
				}
				continue
			}
			if reply.ID == 0 {
				return
			}
			select {
			case r.replies <- reply:
			case <-r.done:
				return
			}
		}
	}()
	return r
}
func (r *rpcClient) Close() {
	r.once.Do(func() {
		close(r.done)
		r.input.Close()
		r.output.Close()
		if r.stop != nil {
			r.stop()
		}
	})
}
func (r *rpcClient) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	r.calls.Lock()
	defer r.calls.Unlock()
	select {
	case <-r.done:
		return nil, errors.New("native protocol closed")
	default:
	}
	r.next++
	id := r.next
	msg, err := json.Marshal(map[string]any{"id": id, "method": method, "params": params})
	if err != nil {
		return nil, err
	}
	written := make(chan error, 1)
	go func() { _, e := r.output.Write(append(msg, '\n')); written <- e }()
	select {
	case err = <-written:
		if err != nil {
			r.Close()
			return nil, errors.New("native protocol write failed")
		}
	case <-ctx.Done():
		r.Close()
		return nil, ctx.Err()
	case <-r.done:
		return nil, errors.New("native protocol closed")
	}
	for {
		select {
		case reply := <-r.replies:
			if reply.ID != id {
				continue
			}
			if reply.Error != nil {
				return nil, &protocolError{code: reply.Error.Code}
			}
			return reply.Result, nil
		case <-ctx.Done():
			r.Close()
			return nil, ctx.Err()
		case <-r.done:
			return nil, errors.New("native protocol closed")
		}
	}
}

type protocolError struct{ code int }

func (e *protocolError) Error() string { return "native account method unavailable or rejected" }
func (r *rpcClient) Initialized(ctx context.Context) error {
	r.calls.Lock()
	defer r.calls.Unlock()
	done := make(chan error, 1)
	go func() { _, err := r.output.Write([]byte("{\"method\":\"initialized\",\"params\":{}}\n")); done <- err }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		r.Close()
		return ctx.Err()
	case <-r.done:
		return errors.New("native protocol closed")
	}
}

type commandRunner interface {
	StartCodex(context.Context, string, string, func(string, json.RawMessage)) (*rpcClient, error)
	Run(context.Context, string, []string) ([]byte, error)
}
type nativeRunner struct{}

func (nativeRunner) StartCodex(ctx context.Context, path, home string, notify func(string, json.RawMessage)) (*rpcClient, error) {
	cmd := exec.CommandContext(ctx, path, "app-server")
	hide(cmd)
	cmd.Env = append(cleanEnvironment(), "CODEX_HOME="+home)
	cmd.Stderr = io.Discard
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		in.Close()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		in.Close()
		out.Close()
		return nil, errors.New("cannot launch Codex app-server")
	}
	waited := make(chan struct{})
	go func() { cmd.Wait(); close(waited) }()
	return newRPC(out, in, func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		select {
		case <-waited:
		case <-time.After(2 * time.Second):
		}
	}, notify), nil
}

type cappedBuffer struct {
	data     []byte
	overflow bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	space := 65536 - len(b.data)
	if len(p) > space {
		b.overflow = true
		p = p[:space]
	}
	b.data = append(b.data, p...)
	return n, nil
}
func (nativeRunner) Run(ctx context.Context, path string, args []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	hide(cmd)
	cmd.Env = cleanEnvironment()
	var b cappedBuffer
	cmd.Stdout = &b
	cmd.Stderr = io.Discard
	err := cmd.Run()
	if err != nil {
		if !b.overflow {
			return b.data, errors.New("native command failed; verify installation and subscription sign-in")
		}
		return nil, errors.New("native output exceeded 64 KiB")
	}
	if b.overflow {
		return nil, errors.New("native output exceeded 64 KiB")
	}
	return b.data, nil
}
func cleanEnvironment() []string {
	var out []string
	for _, entry := range os.Environ() {
		key := entry
		for i, c := range entry {
			if c == '=' {
				key = entry[:i]
				break
			}
		}
		switch strings.ToUpper(key) {
		case "OPENAI_API_KEY", "OPENAI_BASE_URL", "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_BASE_URL", "CLAUDE_CODE_USE_BEDROCK", "CLAUDE_CODE_USE_VERTEX", "CLAUDE_CODE_USE_FOUNDRY", "CODEX_HOME":
			continue
		}
		out = append(out, entry)
	}
	return out
}
