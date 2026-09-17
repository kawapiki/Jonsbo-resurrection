// Package config loads strict local configuration and creates API credentials.
package config

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"github.com/kawapiki/Jonsbo-resurrection/internal/api"
	"github.com/kawapiki/Jonsbo-resurrection/pkg/module"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Module struct {
	Enabled bool            `json:"enabled"`
	Options json.RawMessage `json:"options,omitempty"`
}
type Config struct {
	Listen        string                       `json:"listen"`
	TokenFile     string                       `json:"token_file"`
	FrameInterval string                       `json:"frame_interval"`
	Modules       map[string]Module            `json:"modules"`
	Displays      map[string]module.Assignment `json:"displays"`
}

func Default() Config {
	return Config{Listen: "127.0.0.1:8787", TokenFile: "bin/api-token", FrameInterval: "1s", Modules: map[string]Module{"hardware": {Enabled: true, Options: json.RawMessage(`{"interval":"1s"}`)}, "example": {Enabled: false, Options: json.RawMessage(`{"interval":"1s"}`)}}, Displays: map[string]module.Assignment{}}
}
func Load(path string) (Config, error) {
	c := Default()
	if path != "" {
		f, err := os.Open(path)
		if err != nil {
			return c, err
		}
		defer f.Close()
		data, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
		if err != nil {
			return c, err
		}
		if len(data) > 1<<20 {
			return c, fmt.Errorf("config exceeds 1MiB")
		}
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&c); err != nil {
			return c, err
		}
		if err := dec.Decode(new(any)); err != io.EOF {
			return c, fmt.Errorf("config contains trailing JSON or exceeds 1MiB")
		}
	}
	return c, Validate(c)
}
func Validate(c Config) error {
	if err := api.ValidateListen(c.Listen); err != nil {
		return err
	}
	if c.TokenFile == "" {
		return fmt.Errorf("token_file cannot be empty")
	}
	d, err := time.ParseDuration(c.FrameInterval)
	if err != nil || d < 500*time.Millisecond || d > time.Minute {
		return fmt.Errorf("frame_interval must be 500ms..1m")
	}
	if len(c.Modules) > 32 || len(c.Displays) > 32 {
		return fmt.Errorf("too many modules/displays")
	}
	for id := range c.Modules {
		if !module.ValidID(id) {
			return fmt.Errorf("invalid module id %q", id)
		}
	}
	return nil
}

// Token never replaces an existing credential. Store token files in a directory
// accessible only to the user; Windows applies the directory's inherited ACL.
func Token(path string) (string, error) {
	read := func() (string, error) {
		f, e := os.Open(path)
		if e != nil {
			return "", e
		}
		defer f.Close()
		b, e := io.ReadAll(io.LimitReader(f, 4097))
		if e != nil {
			return "", e
		}
		s := strings.TrimSpace(string(b))
		if len(b) > 4096 || len(s) < 32 || strings.ContainsAny(s, " \t\r\n") {
			return "", fmt.Errorf("token file must contain a single token of 32..4096 characters")
		}
		return s, nil
	}
	if token, err := read(); err == nil {
		return token, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", err
	}
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(b[:])
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if os.IsExist(err) {
		return read()
	}
	if err != nil {
		return "", err
	}
	_, writeErr := io.WriteString(f, token+"\n")
	closeErr := f.Close()
	if writeErr != nil {
		return "", writeErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	return token, nil
}
