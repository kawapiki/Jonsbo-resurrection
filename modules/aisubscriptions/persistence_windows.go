//go:build windows

package aisubscriptions

import (
	"errors"
	"syscall"
)

func transientReplacement(err error) bool {
	// MoveFileEx reports access denied (5) for a destination reader that lacks
	// FILE_SHARE_DELETE. Sharing violation (32) and lock violation (33) can also
	// be transient; persistent ACL/read-only failures remain bounded failures.
	return errors.Is(err, syscall.ERROR_ACCESS_DENIED) || errors.Is(err, syscall.Errno(32)) || errors.Is(err, syscall.Errno(33))
}
