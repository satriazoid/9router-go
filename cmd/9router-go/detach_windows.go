//go:build windows

package main

import "syscall"

// detachedSysProcAttr returns SysProcAttr that detaches the child from this
// console: no console window, own process group, survives parent exit.
func detachedSysProcAttr() *syscall.SysProcAttr {
	const (
		detachedProcess  = 0x00000008
		createNewProcGrp = 0x00000200
		createNoWindow   = 0x08000000
	)
	return &syscall.SysProcAttr{
		HideWindow: true,
		CreationFlags: detachedProcess |
			createNewProcGrp |
			createNoWindow,
	}
}
