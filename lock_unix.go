//go:build !windows

package main

import "syscall"

// processAlive probes a PID with signal 0. EPERM means the process
// exists but belongs to another user; ESRCH means it is gone.
func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
