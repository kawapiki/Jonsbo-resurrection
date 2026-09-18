//go:build windows

package main

import (
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/kawapiki/Jonsbo-resurrection/internal/config"
)

func configuratorURL(dir string) (string, error) {
	cfg := config.Default()
	path := filepath.Join(dir, "config.json")
	if _, err := os.Stat(path); err == nil {
		var e error
		cfg, e = config.Load(path)
		if e != nil {
			return "", e
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	_, port, err := net.SplitHostPort(cfg.Listen)
	if err != nil || port == "0" {
		return "", fmt.Errorf("configurator needs a fixed local listen port")
	}
	f, err := os.Open(filepath.Join(dir, "api-token"))
	if err != nil {
		return "", fmt.Errorf("start monitoring before opening the configurator")
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 4097))
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(b))
	if len(b) > 4096 || len(token) < 32 || strings.ContainsAny(token, " \t\r\n") {
		return "", fmt.Errorf("invalid local API credential")
	}
	u := url.URL{Scheme: "http", Host: cfg.Listen, Path: "/", Fragment: "token=" + url.QueryEscape(token)}
	return u.String(), nil
}
