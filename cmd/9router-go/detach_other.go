//go:build !windows

package main

import "syscall"

// detachedSysProcAttr is a no-op on non-Windows platforms: POSIX daemons
// detach via setsid in the daemon layer; the widget spawn here just inherits
// the parent's session (matching the pre-existing behavior).
func detachedSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}
