//go:build !windows

package main

import (
	"fmt"
	"os"
)

// runWidget is only implemented on Windows (pure Win32, no cgo).
func runWidget(args []string) {
	fmt.Fprintln(os.Stderr, "widget: perintah widget hanya tersedia di Windows")
	os.Exit(1)
}
