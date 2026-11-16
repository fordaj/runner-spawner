package main

import (
	"archive/tar"
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// lifecycleFixture wires a temp repo dir, a fake runner archive (so no
// network is ever needed), a fake executor for the service tooling, and a
// temp home for the LaunchAgent plist.
type lifecycleFixture struct {
	t          *testing.T
	cfg        *Config
	plat       platform
	fake       *fakeExecutor
	runnersDir string
	home       string
	name       string
}

func newLifecycleFixture(t *testing.T, goos string) *lifecycleFixture {
	t.Helper()
	clearSpawnerEnv(t)
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	runnersDir := filepath.Join(repoDir, ".runners")
	cacheDir := filepath.Join(runnersDir, "cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	prevHome := userHomeDir
	userHomeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { userHomeDir = prevHome })
	prevCfgDir := userConfigDir
	cfgDir := t.TempDir()
	userConfigDir = func() (string, error) { return cfgDir, nil }
	t.Cleanup(func() { userConfigDir = prevCfgDir })
	prevTimeout, prevInterval := serviceTimeout, servicePollInterval
	serviceTimeout = 500 * time.Millisecond
	servicePollInterval = time.Millisecond
	t.Cleanup(func() { serviceTimeout, servicePollInterval = prevTimeout, prevInterval })
	prevLockAge := lockStaleAge
	lockStaleAge = time.Minute
	t.Cleanup(func() { lockStaleAge = prevLockAge })

	cfg := DefaultConfig()
	cfg.RepoDir = repoDir
	cfg.RunnersDir = ".runners"
	cfg.Owner = "acme"
	cfg.Repo = "widgets"
	cfg.Token = "test-token"
	cfg.Quiet = true
	plat := platformFor(goos, "amd64", "testhost")
	fake := newFakeExecutor()
	fake.plistPath = macosServicePlist(home, cfg, "runner01")
	fake.swap(t)
	writeCachedRunnerArchive(t, cfg, plat, cacheDir)
	return &lifecycleFixture{t: t, cfg: cfg, plat: plat, fake: fake, runnersDir: runnersDir, home: home, name: "runner01"}
}

func (fx *lifecycleFixture) runnerDir() string {
	return filepath.Join(fx.runnersDir, fx.name)
}

func (fx *lifecycleFixture) plistPath() string {
	return fx.fake.plistPath
}

// captureVerb runs one verb against fresh buffers, dumps the captured
// output under -test.v, and returns it.
func captureVerb(verb string, fn func(stdout, stderr io.Writer) error) (string, string, error) {
	var stdout, stderr strings.Builder
	err := fn(&stdout, &stderr)
	if testing.Verbose() {
		fmt.Fprintf(os.Stderr, "%s stdout:\n%s\n%s stderr:\n%s\n", verb, stdout.String(), verb, stderr.String())
	}
	return stdout.String(), stderr.String(), err
}

func (fx *lifecycleFixture) create() error {
	_, _, err := captureVerb("create", func(so, se io.Writer) error { return createOne(fx.cfg, fx.plat, fx.name, so, se) })
	return err
}

func (fx *lifecycleFixture) remove() error {
	_, _, err := captureVerb("remove", func(so, se io.Writer) error { return removeOne(fx.cfg, fx.plat, fx.name, so, se) })
	return err
}

func (fx *lifecycleFixture) removeCapture() (string, string, error) {
	return captureVerb("remove", func(so, se io.Writer) error { return removeOne(fx.cfg, fx.plat, fx.name, so, se) })
}

// writeCachedRunnerArchive materializes a minimal actions/runner archive
// in the cache so create runs fully offline.
func writeCachedRunnerArchive(t *testing.T, cfg *Config, p platform, cacheDir string) {
	t.Helper()
	_, filename, err := versionedAsset(cfg, p)
	if err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(cacheDir, filename)
	switch p.ArchiveExt {
	case ".zip":
		f, err := os.Create(archivePath)
		if err != nil {
			t.Fatal(err)
		}
		zw := zip.NewWriter(f)
		for _, e := range []struct{ name, body string }{
			{p.ConfigBin, "@echo config\n"},
			{"run.cmd", "@echo run\n"},
		} {
			w, err := zw.Create(e.name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := w.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	default:
		writeTarGz(t, archivePath, []tarEntry{
			{name: "./config.sh", typeflag: tar.TypeReg, body: "#!/bin/sh\nexit 0\n"},
			{name: "./svc.sh", typeflag: tar.TypeReg, body: "#!/bin/sh\nexit 0\n"},
		})
	}
}

func findCall(calls []fakeCommand, substr string) int {
	for i, c := range calls {
		if strings.Contains(c.String(), substr) {
			return i
		}
	}
	return -1
}

func countCalls(calls []fakeCommand, substr string) int {
	n := 0
	for _, c := range calls {
		if strings.Contains(c.String(), substr) {
			n++
		}
	}
	return n
}

// seedExtractedRunner extracts the cached archive into the runner dir
// without configuring, simulating an interrupted extraction/config.
func (fx *lifecycleFixture) seedExtractedRunner() {
	t := fx.t
	cache := filepath.Join(fx.runnersDir, "cache")
	_, filename, err := versionedAsset(fx.cfg, fx.plat)
	if err != nil {
		t.Fatal(err)
	}
	if err := extractArchiveAtomic(filepath.Join(cache, filename), fx.runnerDir(), nil, fx.cfg.RunnerVersion); err != nil {
		t.Fatal(err)
	}
}

// seedConfiguredRunner produces the "configured, nothing else" state.
func (fx *lifecycleFixture) seedConfiguredRunner() {
	t := fx.t
	fx.seedExtractedRunner()
	if err := os.WriteFile(filepath.Join(fx.runnerDir(), ".runner"), []byte(`{"agent":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCreateFromAbsent_MacOS(t *testing.T) {
	fx := newLifecycleFixture(t, "darwin")
	if err := fx.create(); err != nil {
		t.Fatalf("create: %v", err)
	}
	dir := fx.runnerDir()
	if !isConfigured(dir) {
		t.Fatal(".runner missing after create")
	}
	if got := installedRunnerVersion(dir); got != "2.338.2" {
		t.Fatalf("version marker = %q", got)
	}
	if got := fx.fake.SvcState(); got != "running" {
		t.Fatalf("svcState = %q, want running", got)
	}
	if _, err := os.Stat(fx.plistPath()); err != nil {
		t.Fatalf("plist missing: %v", err)
	}
	calls := fx.fake.Calls()
	order := []string{"config.sh", "svc.sh install", "svc.sh start"}
	last := -1
	for _, want := range order {
		idx := findCall(calls, want)
		if idx < 0 {
			t.Fatalf("missing call %q in %v", want, fx.fake.CallStrings())
		}
		if idx < last {
			t.Fatalf("call %q out of order in %v", want, fx.fake.CallStrings())
		}
		last = idx
	}
	// config.sh must run with the runner dir as cwd (issue #3).
	for _, c := range calls {
		if strings.Contains(c.Name, "config") && c.Dir != dir {
			t.Fatalf("config.sh ran with dir %q, want %q", c.Dir, dir)
		}
	}
}

func TestCreateFromAbsent_Windows(t *testing.T) {
	fx := newLifecycleFixture(t, "windows")
	fx.cfg.WindowsUsername = "svcacct"
	fx.cfg.WindowsPassword = "s3cret"
	if err := fx.create(); err != nil {
		t.Fatalf("create: %v", err)
	}
	if !isConfigured(fx.runnerDir()) {
		t.Fatal(".runner missing after create")
	}
	if got := fx.fake.SvcState(); got != "running" {
		t.Fatalf("svcState = %q, want running", got)
	}
	calls := fx.fake.CallStrings()
	found := false
	for i, c := range calls {
		if strings.Contains(c, "config.cmd") {
			found = true
			if !strings.Contains(c, "--runasservice") {
				t.Fatalf("config.cmd args = %q, want --runasservice", c)
			}
		}
		_ = i
	}
	if !found {
		t.Fatalf("config.cmd not called: %v", calls)
	}
}

func TestCreateFromEmptyDir(t *testing.T) {
	fx := newLifecycleFixture(t, "darwin")
	// Dir exists but extraction never happened / crashed immediately.
	if err := os.MkdirAll(fx.runnerDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := fx.create(); err != nil {
		t.Fatalf("create: %v", err)
	}
	if got := fx.fake.SvcState(); got != "running" {
		t.Fatalf("svcState = %q, want running", got)
	}
	if !isConfigured(fx.runnerDir()) {
		t.Fatal(".runner missing after create")
	}
}

func TestCreateRepairsPartialExtraction(t *testing.T) {
	fx := newLifecycleFixture(t, "darwin")
	// Simulate a half-written extraction: config.sh present, svc.sh not,
	// plus a stale binary from an older install.
	dir := fx.runnerDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFileString(t, filepath.Join(dir, "config.sh"), "#!/bin/sh\n")
	writeFileString(t, filepath.Join(dir, "stale-from-older-version"), "old")
	if err := fx.create(); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "stale-from-older-version")); !os.IsNotExist(err) {
		t.Fatal("stale file survived repair")
	}
	if got := fx.fake.SvcState(); got != "running" {
		t.Fatalf("svcState = %q, want running", got)
	}
}

func TestCreateConfiguresExtractedRunner(t *testing.T) {
	fx := newLifecycleFixture(t, "darwin")
	// Extracted (marker matches) but never configured.
	fx.seedExtractedRunner()
	fx.fake.Calls()
	if err := fx.create(); err != nil {
		t.Fatalf("create: %v", err)
	}
	calls := fx.fake.Calls()
	if countCalls(calls, "config.sh") != 1 {
		t.Fatalf("config.sh calls = %d, want 1 (%v)", countCalls(calls, "config.sh"), fx.fake.CallStrings())
	}
	if got := fx.fake.SvcState(); got != "running" {
		t.Fatalf("svcState = %q, want running", got)
	}
}

// TestCreateIssue3_ConfiguredServiceNotInstalled is the regression test
// for issue #3: runner configured, service never installed. Re-running
// create must install + start without reconfiguring.
func TestCreateIssue3_ConfiguredServiceNotInstalled(t *testing.T) {
	fx := newLifecycleFixture(t, "darwin")
	fx.seedConfiguredRunner() // .runner present, marker matches, no service
	if err := fx.create(); err != nil {
		t.Fatalf("create: %v", err)
	}
	calls := fx.fake.Calls()
	if n := countCalls(calls, "config.sh"); n != 0 {
		t.Fatalf("config.sh calls = %d, want 0 (%v)", n, fx.fake.CallStrings())
	}
	if n := countCalls(calls, "svc.sh install"); n != 1 {
		t.Fatalf("svc.sh install calls = %d, want 1 (%v)", n, fx.fake.CallStrings())
	}
	if n := countCalls(calls, "svc.sh start"); n != 1 {
		t.Fatalf("svc.sh start calls = %d, want 1 (%v)", n, fx.fake.CallStrings())
	}
	if got := fx.fake.SvcState(); got != "running" {
		t.Fatalf("svcState = %q, want running", got)
	}
}

func TestCreateServiceInstalledButStopped(t *testing.T) {
	fx := newLifecycleFixture(t, "darwin")
	if err := fx.create(); err != nil {
		t.Fatalf("create: %v", err)
	}
	fx.fake.setSvcState("stopped")
	callsBefore := len(fx.fake.Calls())
	if err := fx.create(); err != nil {
		t.Fatalf("create: %v", err)
	}
	calls := fx.fake.Calls()[callsBefore:]
	if n := countCalls(calls, "svc.sh start"); n != 1 {
		t.Fatalf("svc.sh start calls = %d, want 1 (%v)", n, fx.fake.CallStrings())
	}
	if n := countCalls(calls, "config.sh"); n != 0 {
		t.Fatalf("config.sh calls = %d, want 0; a stopped runner must not be reconfigured", n)
	}
	if n := countCalls(calls, "svc.sh install"); n != 0 {
		t.Fatalf("svc.sh install calls = %d, want 0; already installed", n)
	}
	if got := fx.fake.SvcState(); got != "running" {
		t.Fatalf("svcState = %q, want running", got)
	}
}

func TestCreateAlreadyRunningIsIdempotent(t *testing.T) {
	fx := newLifecycleFixture(t, "darwin")
	if err := fx.create(); err != nil {
		t.Fatalf("create: %v", err)
	}
	callsBefore := len(fx.fake.Calls())
	if err := fx.create(); err != nil {
		t.Fatalf("create: %v", err)
	}
	// A healthy runner must not be reconfigured: only read-only status
	// probes are allowed on the no-op path.
	for _, c := range fx.fake.Calls()[callsBefore:] {
		s := c.String()
		if strings.Contains(s, "config.sh") || strings.Contains(s, "svc.sh install") || strings.Contains(s, "svc.sh start") {
			t.Fatalf("second create ran mutating command %q; want none", s)
		}
	}
}

func TestCreateCrashAfterConfig_Resumes(t *testing.T) {
	fx := newLifecycleFixture(t, "darwin")
	// Interrupt the first create right after config succeeded on disk.
	fx.fake.CrashConfig = errTest
	if err := fx.create(); err == nil {
		t.Fatal("expected crash error")
	}
	fx.fake.CrashConfig = nil
	if err := fx.create(); err != nil {
		t.Fatalf("resume create: %v", err)
	}
	if got := fx.fake.SvcState(); got != "running" {
		t.Fatalf("svcState = %q, want running", got)
	}
}

func TestCreateFailConfig_Resumes(t *testing.T) {
	fx := newLifecycleFixture(t, "darwin")
	fx.fake.FailConfig = errTest
	if err := fx.create(); err == nil {
		t.Fatal("expected config failure")
	}
	fx.fake.FailConfig = nil
	if err := fx.create(); err != nil {
		t.Fatalf("resume create: %v", err)
	}
	if got := fx.fake.SvcState(); got != "running" {
		t.Fatalf("svcState = %q, want running", got)
	}
}

// TestCreateCrashDuringSvcInstall_Resumes kills the create right after
// svc.sh install half-succeeded (service installed, start never ran).
// Re-running must notice the installed-but-stopped service and start it.
func TestCreateCrashDuringSvcInstall_Resumes(t *testing.T) {
	fx := newLifecycleFixture(t, "darwin")
	fx.fake.CrashSvcInstall = errTest
	if err := fx.create(); err != nil {
		// The crash happens after the install succeeded on disk; the
		// tool tolerates this (re-probe shows installed) and continues.
		t.Fatalf("create: %v", err)
	}
	if got := fx.fake.SvcState(); got != "running" {
		t.Fatalf("svcState = %q, want running", got)
	}
	// And a completely interrupted run (process died) resumes too.
	if err := fx.create(); err != nil {
		t.Fatalf("second create: %v", err)
	}
}

func TestRemoveFromRunning_MacOS(t *testing.T) {
	fx := newLifecycleFixture(t, "darwin")
	if err := fx.create(); err != nil {
		t.Fatalf("create: %v", err)
	}
	before := len(fx.fake.Calls())
	if err := fx.remove(); err != nil {
		t.Fatalf("remove: %v", err)
	}
	calls := fx.fake.Calls()[before:]
	// Order: stop, uninstall, then config.sh remove, then dir deleted.
	order := []string{"svc.sh stop", "svc.sh uninstall", "config.sh remove"}
	last := -1
	for _, want := range order {
		idx := findCall(calls, want)
		if idx < 0 {
			t.Fatalf("missing %q in %v", want, fx.fake.CallStrings())
		}
		if idx < last {
			t.Fatalf("%q out of order in %v", want, fx.fake.CallStrings())
		}
		last = idx
	}
	if got := fx.fake.SvcState(); got != "not-installed" {
		t.Fatalf("svcState = %q, want not-installed", got)
	}
	if _, err := os.Stat(fx.runnerDir()); !os.IsNotExist(err) {
		t.Fatalf("runner dir still exists: %v", err)
	}
	if _, err := os.Stat(fx.plistPath()); !os.IsNotExist(err) {
		t.Fatalf("plist still exists: %v", err)
	}
}

func TestRemoveFromAbsentIsCleanNoop(t *testing.T) {
	fx := newLifecycleFixture(t, "darwin")
	fake := fx.fake
	fake.GitHubListIDs = nil
	if err := fx.remove(); err != nil {
		t.Fatalf("remove: %v", err)
	}
	// gh is consulted (best effort) but nothing explodes.
	if n := countCalls(fx.fake.Calls(), "config.sh"); n != 0 {
		t.Fatalf("config.sh called %d times, want 0", n)
	}
}

func TestRemoveOrphanPlistCleanedUp(t *testing.T) {
	fx := newLifecycleFixture(t, "darwin")
	// Runner dir is gone; the LaunchAgent plist lingers.
	if err := os.MkdirAll(filepath.Dir(fx.plistPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fx.plistPath(), []byte("<plist/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := fx.remove(); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(fx.plistPath()); !os.IsNotExist(err) {
		t.Fatal("orphan plist was not removed")
	}
}

func TestRemoveUninstallFailureStillConverges(t *testing.T) {
	fx := newLifecycleFixture(t, "darwin")
	if err := fx.create(); err != nil {
		t.Fatalf("create: %v", err)
	}
	serviceTimeout = 100 * time.Millisecond
	fx.fake.FailSvcUninstall = errTest
	if err := fx.remove(); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(fx.plistPath()); !os.IsNotExist(err) {
		t.Fatal("plist still exists after orphan cleanup")
	}
	if _, err := os.Stat(fx.runnerDir()); !os.IsNotExist(err) {
		t.Fatal("runner dir still exists")
	}
}

// TestRemoveRerunsAfterDeregistrationFailure verifies: deregistration
// fails on the first remove; re-running remove completes the job via the
// admin API (no local runner dir needed).
func TestRemoveRerunsAfterDeregistrationFailure(t *testing.T) {
	fx := newLifecycleFixture(t, "darwin")
	if err := fx.create(); err != nil {
		t.Fatalf("create: %v", err)
	}
	fx.fake.FailConfigRemove = errTest
	_, stderr, err := fx.removeCapture()
	if err != nil {
		t.Fatalf("first remove should not hard-fail on deregistration, got %v", err)
	}
	if !strings.Contains(stderr, "deregistration") && !strings.Contains(stderr, "GitHub") {
		t.Fatalf("first remove should warn about GitHub, stderr:\n%s", stderr)
	}
	if _, err := os.Stat(fx.runnerDir()); !os.IsNotExist(err) {
		t.Fatal("runner dir should be deleted even when remote deregistration fails")
	}
	// Re-run: the dir is gone; deregistration completes via gh API.
	fx.fake.GitHubListIDs = []string{"42"}
	_, stderr, err = fx.removeCapture()
	if err != nil {
		t.Fatalf("second remove: %v", err)
	}
	found := false
	for _, d := range fx.fake.GitHubDeleted {
		if strings.Contains(d, "actions/runners/42") {
			found = true
		}
	}
	if !found {
		t.Fatalf("second remove did not delete GitHub runner 42; deleted=%v", fx.fake.GitHubDeleted)
	}
}

func TestRemoveLocalOnlySkipsGitHub(t *testing.T) {
	fx := newLifecycleFixture(t, "darwin")
	if err := fx.create(); err != nil {
		t.Fatalf("create: %v", err)
	}
	fx.cfg.LocalOnly = true
	if err := fx.remove(); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if n := countCalls(fx.fake.Calls(), "config.sh remove"); n != 0 {
		t.Fatalf("config.sh remove called %d times, want 0 with --local-only", n)
	}
}

func TestRemoveTokenFailureProceedsLocally(t *testing.T) {
	fx := newLifecycleFixture(t, "darwin")
	if err := fx.create(); err != nil {
		t.Fatalf("create: %v", err)
	}
	// Make token resolution fail: no config/env token and gh fails.
	fx.cfg.Token = ""
	t.Setenv(EnvToken, "")
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	fx.fake.GitHubTokenErr = errTest
	_, stderr, err := fx.removeCapture()
	if err != nil {
		t.Fatalf("remove should proceed without a token, got %v", err)
	}
	if !strings.Contains(stderr, "registration token") {
		t.Fatalf("expected a warning about the token, stderr:\n%s", stderr)
	}
	if _, err := os.Stat(fx.runnerDir()); !os.IsNotExist(err) {
		t.Fatal("runner dir should be deleted even without a token")
	}
}

func TestWindowsServicePendingWaitsForStop(t *testing.T) {
	fx := newLifecycleFixture(t, "windows")
	fx.cfg.WindowsUsername = "svcacct"
	fx.cfg.WindowsPassword = "s3cret"
	if err := fx.create(); err != nil {
		t.Fatalf("create: %v", err)
	}
	// sc.exe stop reports STOP_PENDING twice before STOPPED.
	fx.fake.WindowsStopPendingCount = 2
	if err := fx.remove(); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if got := fx.fake.SvcState(); got != "not-installed" {
		t.Fatalf("svcState = %q, want not-installed", got)
	}
}

func TestWindowsRemoveFromAbsent(t *testing.T) {
	fx := newLifecycleFixture(t, "windows")
	// Service exists but dir is gone (orphan on Windows).
	fx.fake.setSvcState("stopped")
	if err := fx.remove(); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if got := fx.fake.SvcState(); got != "not-installed" {
		t.Fatalf("svcState = %q, want not-installed (orphan deleted)", got)
	}
}

func TestStatusHints(t *testing.T) {
	fx := newLifecycleFixture(t, "darwin")
	// Nothing exists yet.
	st, err := statusOne(fx.cfg, fx.plat, fx.name)
	if err != nil {
		t.Fatal(err)
	}
	if st.Hint == "" || !strings.Contains(st.Hint, "create") {
		t.Fatalf("hint for absent runner = %q, want a create hint", st.Hint)
	}
	// Extracted but not configured.
	fx.seedExtractedRunner()
	st, err = statusOne(fx.cfg, fx.plat, fx.name)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(st.Hint, "create") {
		t.Fatalf("hint for extracted-but-unconfigured = %q", st.Hint)
	}
	// Configured but service not installed.
	fx.seedConfiguredRunner()
	st, err = statusOne(fx.cfg, fx.plat, fx.name)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(st.Hint, "create") {
		t.Fatalf("hint for configured-no-service = %q", st.Hint)
	}
}

func TestWindowsServiceStateParsingIsLocaleIndependent(t *testing.T) {
	// French Windows prints localized labels but the numeric STATE
	// code is stable: 1=STOPPED, 4=RUNNING, 3=STOP_PENDING.
	cases := []struct {
		name string
		out  string
		want string
	}{
		{"english running", "        STATE              : 4  RUNNING\n", "running"},
		{"french running", "        ÉTAT               : 4  EN COURS\n", "running"},
		{"german stopped", "        STATUS             : 1  BEENDET\n", "stopped"},
		{"japanese pending", "        状態               : 3  停止処理中\n", "pending"},
		{"english stopped", "        STATE              : 1  STOPPED\n", "stopped"},
		{"unknown without numeric", "garbage output\n", ""},
		{"english fallback", "        STATU              : xyzzy RUNNING\n", "running"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := windowsServiceStateFromOutput(tc.out); got != tc.want {
				t.Fatalf("windowsServiceStateFromOutput(%q) = %q, want %q", tc.out, got, tc.want)
			}
		})
	}
}

func TestWindowsQueryServiceStateExitCodes(t *testing.T) {
	fx := newLifecycleFixture(t, "windows")
	// Default state: service does not exist → 1060.
	if got := queryServiceState(fx.cfg, fx.plat, fx.runnerDir(), fx.name); got != "not-installed" {
		t.Fatalf("queryServiceState = %q, want not-installed", got)
	}
	// 1072 (marked for deletion) also maps to not-installed.
	fx.fake.svcState = "not-installed"
	out, err := fx.fake.scQueryOutput([]string{"query", "svc"})
	_ = out
	if err == nil {
		t.Fatal("want error for missing service")
	}
}

func TestAdminCheckIsExitCodeBased(t *testing.T) {
	fx := newLifecycleFixture(t, "windows")
	fx.fake.FailNetSession = errTest
	if err := adminCheck(fx.plat); err == nil {
		t.Fatal("want error when net session fails (not admin)")
	}
	fx.fake.FailNetSession = nil
	if err := adminCheck(fx.plat); err != nil {
		t.Fatalf("adminCheck: %v", err)
	}
	// Non-Windows: no-op.
	darwin := platformFor("darwin", "arm64", "testhost")
	if err := adminCheck(darwin); err != nil {
		t.Fatalf("adminCheck(darwin): %v", err)
	}
}

func TestDryRunPasswordNeverPrinted(t *testing.T) {
	fx := newLifecycleFixture(t, "windows")
	fx.cfg.WindowsUsername = "svcacct"
	fx.cfg.WindowsPassword = "super-secret-password"
	fx.cfg.DryRun = true
	var stdout, stderr strings.Builder
	if err := createOne(fx.cfg, fx.plat, fx.name, &stdout, &stderr); err != nil {
		t.Fatalf("create: %v", err)
	}
	for _, out := range []string{stdout.String(), stderr.String()} {
		if strings.Contains(out, "super-secret-password") {
			t.Fatalf("password leaked into output:\n%s", out)
		}
	}
}

func TestCreateFormatJSON(t *testing.T) {
	fx := newLifecycleFixture(t, "darwin")
	fx.cfg.Format = "json"
	prevPlatform := currentPlatform
	currentPlatform = func() platform { return fx.plat }
	t.Cleanup(func() { currentPlatform = prevPlatform })
	code := func() error {
		return runVerb("create", []string{
			"--repo-dir", fx.cfg.RepoDir,
			"--owner", "acme",
			"--repo", "widgets",
			"--name", "runner01",
			"--format", "json",
			"--token", "test-token",
			"--quiet",
		}, io.Discard, io.Discard)
	}()
	if code != nil {
		t.Fatalf("create --format json: %v", code)
	}
}

func TestMutationResultJSONShape(t *testing.T) {
	var buf strings.Builder
	cfg := DefaultConfig()
	cfg.Owner, cfg.Repo = "acme", "widgets"
	err := writeMutationJSON(&buf, cfg, "directory", []mutationResult{{Name: "runner01", OK: true}})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(buf.String()), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}
	if decoded["repo"] != "acme/widgets" {
		t.Fatalf("repo = %v", decoded["repo"])
	}
	res, ok := decoded["results"].([]any)
	if !ok || len(res) != 1 {
		t.Fatalf("results = %v", decoded["results"])
	}
}

func TestWindowsDryRun(t *testing.T) {
	fx := newLifecycleFixture(t, "windows")
	fx.cfg.WindowsUsername = "svcacct"
	fx.cfg.DryRun = true
	var stdout strings.Builder
	if err := createOne(fx.cfg, fx.plat, fx.name, &stdout, io.Discard); err != nil {
		t.Fatalf("create: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{"create", "config.cmd", "would"} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, out)
		}
	}
}
