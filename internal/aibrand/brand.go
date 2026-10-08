// Package aibrand embeds the providers' public documentation icons.
package aibrand

import (
	"bytes"
	_ "embed"
	"image"
	"image/png"
	"net/http"
	"sync"
)

//go:embed openai.png
var openai []byte

//go:embed claude.png
var claude []byte

var once sync.Once
var marks map[string]image.Image

// Logo returns an immutable image. Callers must not modify its pixels.
func Logo(provider string) image.Image {
	once.Do(func() {
		marks = make(map[string]image.Image)
		for name, b := range map[string][]byte{"openai": openai, "claude": claude} {
			img, err := png.Decode(bytes.NewReader(b))
			if err == nil {
				marks[name] = img
			}
		}
	})
	return marks[provider]
}

// ServeHTTP serves only the two embedded public assets.
func ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var b []byte
	switch r.URL.Path {
	case "/ai-assets/openai.png":
		b = openai
	case "/ai-assets/claude.png":
		b = claude
	default:
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method Not Allowed", 405)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	if r.Method == http.MethodGet {
		_, _ = w.Write(b)
	}
}
