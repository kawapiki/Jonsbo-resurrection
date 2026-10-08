//go:build !windows

package aisubscriptions

func transientReplacement(error) bool { return false }
