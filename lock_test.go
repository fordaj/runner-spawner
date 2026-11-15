package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreateStaleVersionReExtractPreservesConfig(t *testing.T) {
	fx := newLifecycleFixture(t, "darwin")
	// Simulate an older install: everything present, but the version
	// marker is stale and there are files the new archive doesn't ship.
	fx.seedConfiguredRunner()
	dir := fx.runnerDir()
	writeFileString(t, filepath.Join(dir, "stale-from-older-version"), "old")
	writeFileString(t, filepath.Join(dir, versionMarkerFile), "2.300.0\n")

	if err := fx.create(); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "stale-from-older-version")); !os.IsNotExist(err) {
		t.Fatal("stale file survived re-extract")
	}
	// The configured state must survive the version swap: config.sh is
	// not re-run.
	if !isConfigured(dir) {
		t.Fatal(".runner lost during re-extract")
	}
	calls := fx.fake.Calls()
	if n := countCalls(calls, "config.sh"); n != 0 {
		t.Fatalf("config.sh calls = %d, want 0 (config must survive a version swap)", n)
	}
	if got := installedRunnerVersion(dir); got != "2.338.2" {
		t.Fatalf("version marker = %q, want 2.338.2", got)
	}
	if got := fx.fake.SvcState(); got != "running" {
		t.Fatalf("svcState = %q, want running", got)
	}
}

func TestLockRunnerSerializes(t *testing.T) {
	root := t.TempDir()
	unlock1, err := lockRunner(root, "runner01")
	if err != nil {
		t.Fatalf("first lock: %v", err)
	}
	defer unlock1()
	if _, err := lockRunner(root, "runner01"); err == nil {
		t.Fatal("second lock should fail while the first is held")
	} else if !strings.Contains(err.Error(), "pid") {
		t.Fatalf("error should name the owning pid, got %q", err)
	}
	// A different runner is unaffected.
	unlock2, err := lockRunner(root, "runner02")
	if err != nil {
		t.Fatalf("lock for other runner: %v", err)
	}
	unlock2()
	// After release, re-acquiring works.
	unlock1()
	unlock3, err := lockRunner(root, "runner01")
	if err != nil {
		t.Fatalf("re-acquire after release: %v", err)
	}
	unlock3()
}

func TestLockRunnerStaleTakeover(t *testing.T) {
	root := t.TempDir()
	locksDir := filepath.Join(root, ".locks")
	if err := os.MkdirAll(locksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(locksDir, "runner01.lock")
	// A lock from a crashed process, old enough to be stale.
	if err := os.WriteFile(lockPath, []byte("pid=999999\ncreated=2006-01-02T15:04:05Z\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * lockStaleAge)
	if err := os.Chtimes(lockPath, old, old); err != nil {
		t.Fatal(err)
	}
	unlock, err := lockRunner(root, "runner01")
	if err != nil {
		t.Fatalf("stale lock should be taken over: %v", err)
	}
	unlock()
}

func TestLockRunnerUnparseableLockTakenOver(t *testing.T) {
	root := t.TempDir()
	locksDir := filepath.Join(root, ".locks")
	if err := os.MkdirAll(locksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(locksDir, "runner01.lock")
	if err := os.WriteFile(lockPath, []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	unlock, err := lockRunner(root, "runner01")
	if err != nil {
		t.Fatalf("unparseable lock should be treated as stale: %v", err)
	}
	unlock()
}
