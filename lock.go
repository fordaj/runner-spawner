package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// lockStaleAge is how old a lock may get before another process may take
// it over. An interrupted create/remove can leave a lock behind; the age
// cap guarantees the runner converges instead of wedging forever.
var lockStaleAge = 15 * time.Minute

// lockRunner serializes mutating verbs on one runner using a
// <runnersRoot>/.locks/<name>.lock file. The returned function releases
// the lock. A stale lock (older than lockStaleAge, or owned by a dead
// process where the OS lets us probe it) is taken over automatically.
func lockRunner(runnersRoot, name string) (func(), error) {
	locksDir := filepath.Join(runnersRoot, ".locks")
	if err := os.MkdirAll(locksDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating lock dir %s: %w", locksDir, err)
	}
	path := filepath.Join(locksDir, name+".lock")
	if err := acquireLockFile(path, name); err != nil {
		return nil, err
	}
	return func() { _ = os.Remove(path) }, nil
}

func acquireLockFile(path, name string) error {
	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_, _ = fmt.Fprintf(f, "pid=%d\ncreated=%s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339Nano))
			_ = f.Close()
			return nil
		}
		if !os.IsExist(err) {
			return fmt.Errorf("creating lock %s: %w", path, err)
		}
		stale, pid, _ := lockIsStale(path)
		if !stale {
			return fmt.Errorf("another runner-spawner process (pid %d, lock %s) is operating on runner %q; wait for it to finish, or remove the lock if that process is dead", pid, path, name)
		}
		_ = os.Remove(path)
	}
	return fmt.Errorf("could not acquire lock %s: lock still present after stale takeover", path)
}

func lockIsStale(path string) (stale bool, pid int, age time.Duration) {
	data, err := os.ReadFile(path)
	if err != nil {
		return true, 0, 0
	}
	content := string(data)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "pid=") {
			pid, _ = strconv.Atoi(strings.TrimPrefix(line, "pid="))
		}
	}
	// Prefer the mtime of the lock file for age: clock skew between the
	// two timestamps doesn't matter for a "how long has this been wedged"
	// question.
	if fi, err := os.Stat(path); err == nil {
		age = time.Since(fi.ModTime())
	}
	if age > lockStaleAge {
		return true, pid, age
	}
	if pid > 0 && !processAlive(pid) {
		return true, pid, age
	}
	// Unparseable or empty lock: treat as stale so an interrupted run
	// never wedges the runner.
	if pid == 0 {
		return true, pid, age
	}
	return false, pid, age
}
