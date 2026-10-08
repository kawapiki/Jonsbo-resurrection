//go:build !windows

package aiadapters

import "os/exec"

func hide(cmd *exec.Cmd) {}
