//go:build windows

package main

import "testing"

func TestChildControlName(t *testing.T) {
	for _, name := range []string{"", `Local\JonsboResurrectionChild-123`} {
		if !validChildControlName(name) {
			t.Fatalf("valid name rejected %q", name)
		}
	}
	for _, name := range []string{`Local\OtherApp`, `Local\JonsboResurrectionChild-`, `Local\JonsboResurrectionChild-1\bad`, `Global\JonsboResurrectionChild-1`} {
		if validChildControlName(name) {
			t.Fatalf("unsafe name accepted %q", name)
		}
	}
}
