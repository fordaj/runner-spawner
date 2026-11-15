//go:build windows

package main

// processAlive on Windows: OpenProcess via the Win32 API would require an
// extra dependency for a hint-only check, so we conservatively report
// "alive" and rely on the lock age cap (lockStaleAge) for stale-lock
// takeover.
func processAlive(pid int) bool {
	return true
}
