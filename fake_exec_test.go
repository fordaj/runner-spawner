package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fakeCommand records one invocation seen by fakeExecutor.
type fakeCommand struct {
	Name string
	Args []string
	Dir  string
}

func (c fakeCommand) String() string {
	return strings.Join(append([]string{c.Name}, c.Args...), " ")
}

// fakeExitError implements ExitCode so the code under test can use the
// exitCoder interface the same way it does with *exec.ExitError.
type fakeExitError struct {
	code int
	msg  string
}

func (e *fakeExitError) Error() string {
	if e.msg != "" {
		return e.msg
	}
	return fmt.Sprintf("exit code %d", e.code)
}

func (e *fakeExitError) ExitCode() int { return e.code }

// fakeExecutor is an in-memory stand-in for the Actions runner scripts and
// Windows service tooling. It records every invocation and simulates the
// on-disk and service-registry state transitions the real scripts perform,
// so lifecycle tests can assert both the command sequence and the end
// state — cross-platform, with no network and no real services.
//
// Failure injection: set any of the Fail* fields to an error; the matching
// step then returns it and does not apply its state transition. Crash*
// variants apply the transition and *then* return the error, simulating an
// interruption mid-step.
type fakeExecutor struct {
	mu sync.Mutex

	calls []fakeCommand

	// svcState is the simulated service state:
	// "running", "stopped" or "not-installed" (default not-installed).
	svcState string
	// plistPath, when set, is the LaunchAgent plist file the fake creates
	// on `svc.sh install` and removes on `svc.sh uninstall`. When empty,
	// the fake simulates service state without touching the filesystem
	// (state machine only).
	plistPath string

	// configured tracks the simulated .runner file, persisted at
	// <dir>/.runner so the code under test sees it via isConfigured.
	configured map[string]bool

	// gh CLI simulation (token minting + admin API for re-registration
	// and deregistration when the dir is gone).
	GitHubTokenErr    error
	GitHubListIDs     []string
	GitHubListErr     error
	GitHubDeleteErr   error
	GitHubDeleted     []string
	FailConfig        error // config.sh/config.cmd configure step
	CrashConfig       error // config succeeded on disk, then "crashed"
	FailConfigRemove  error // config.sh remove step
	FailSvcInstall    error
	CrashSvcInstall   error // service marked installed, then "crashed"
	FailSvcStart      error
	FailSvcStop       error
	FailSvcUninstall  error
	FailSvcStatus     error // svc.sh status probe fails outright
	FailScQuery       error // sc.exe query fails for a non-1060 reason
	FailNetSession    error // Windows admin probe fails
	FailScStop        error // sc.exe stop fails
	FailScDelete      error // sc.exe delete fails
	FailConfigMissing error // config.sh absent: command not found-style error

	// WindowsStopPendingCount, when > 0, makes the simulated service
	// report STOP_PENDING (sc.exe query shows state 3) that many times
	// after `sc.exe stop` before settling to STOPPED. Used to exercise
	// wait-for-stopped polling.
	WindowsStopPendingCount int
	stopPending             int
}

func newFakeExecutor() *fakeExecutor {
	return &fakeExecutor{svcState: "not-installed"}
}

// swap installs f as the command executor and restores the real one at the
// end of the test.
func (f *fakeExecutor) swap(t *testing.T) {
	t.Helper()
	prev := commandExecution
	commandExecution = f
	t.Cleanup(func() { commandExecution = prev })
}

func (f *fakeExecutor) record(name string, args []string, dir string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCommand{Name: name, Args: append([]string(nil), args...), Dir: dir})
}

// Calls returns a copy of the recorded invocations.
func (f *fakeExecutor) Calls() []fakeCommand {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fakeCommand(nil), f.calls...)
}

// CallStrings returns Calls rendered as "name args..." strings for easy
// order assertions.
func (f *fakeExecutor) CallStrings() []string {
	calls := f.Calls()
	out := make([]string, 0, len(calls))
	for _, c := range calls {
		out = append(out, c.String())
	}
	return out
}

// SvcState returns the simulated service state.
func (f *fakeExecutor) SvcState() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.svcState
}

func (f *fakeExecutor) setSvcState(s string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.svcState = s
}

func baseName(name string) string {
	base := filepath.Base(name)
	// Case-insensitive match for Windows .cmd scripts.
	return base
}

func (f *fakeExecutor) writePlist(v bool) {
	if f.plistPath == "" {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if v {
		_ = os.MkdirAll(filepath.Dir(f.plistPath), 0o755)
		_ = os.WriteFile(f.plistPath, []byte("<plist></plist>"), 0o644)
		return
	}
	_ = os.Remove(f.plistPath)
}

func isConfigRemove(args []string) bool {
	for _, a := range args {
		if a == "remove" {
			return true
		}
	}
	return false
}

func (f *fakeExecutor) Run(name string, args []string, dir string) error {
	f.record(name, args, dir)
	_, err := f.execute(name, args, dir)
	return err
}

func (f *fakeExecutor) Output(name string, args []string, dir string) (string, error) {
	f.record(name, args, dir)
	return f.execute(name, args, dir)
}

// execute is the single state-machine behind Run and Output: every
// command applied through the executor mutates the simulated on-disk and
// service state exactly once, regardless of whether the caller captures
// output.
func (f *fakeExecutor) execute(name string, args []string, dir string) (string, error) {
	base := baseName(name)
	base = strings.ToLower(base)
	switch base {
	case "config.sh", "config.cmd":
		if err := f.runConfig(args, dir); err != nil {
			return err.Error(), err
		}
		return "", nil
	case "svc.sh":
		return f.svcAction(args, dir)
	case "sc.exe":
		return f.scAction(args)
	case "net":
		if f.FailNetSession != nil {
			return "", f.FailNetSession
		}
		return "", nil
	case "gh":
		return f.runGH(args)
	case "launchctl":
		// launchctl unload -w <plist>: the unload itself may fail (job
		// not loaded); the plist removal is done by the caller.
		return "", nil
	default:
		return "", fmt.Errorf("fakeExecutor: unexpected command %q", name)
	}
}

// runGH simulates the gh CLI calls runner-spawner makes: minting a
// registration token and listing/deleting repo-scoped runners.
func (f *fakeExecutor) runGH(args []string) (string, error) {
	joined := strings.Join(args, " ")
	switch {
	case strings.Contains(joined, "registration-token"):
		if f.GitHubTokenErr != nil {
			return "", f.GitHubTokenErr
		}
		return "fake-registration-token\n", nil
	case strings.Contains(joined, "actions/runners") && strings.Contains(joined, "DELETE"):
		if f.GitHubDeleteErr != nil {
			return "", f.GitHubDeleteErr
		}
		f.mu.Lock()
		f.GitHubDeleted = append(f.GitHubDeleted, joined)
		f.mu.Unlock()
		return "", nil
	case strings.Contains(joined, "actions/runners"):
		if f.GitHubListErr != nil {
			return "", f.GitHubListErr
		}
		f.mu.Lock()
		ids := append([]string(nil), f.GitHubListIDs...)
		f.mu.Unlock()
		return strings.Join(ids, "\n") + "\n", nil
	default:
		return "", fmt.Errorf("fakeExecutor: unexpected gh call %q", joined)
	}
}

func (f *fakeExecutor) runConfig(args []string, dir string) error {
	if isConfigRemove(args) {
		if f.FailConfigRemove != nil {
			return f.FailConfigRemove
		}
		// Deregistration removes the .runner file on GitHub's side and
		// locally.
		f.writeRunner(dir, false)
		return nil
	}
	if f.FailConfig != nil {
		return f.FailConfig
	}
	// Successful configuration writes .runner and (on Windows with
	// --runasservice) installs the service.
	f.writeRunner(dir, true)
	for _, a := range args {
		if a == "--runasservice" {
			f.setSvcState("stopped")
		}
	}
	if f.CrashConfig != nil {
		return f.CrashConfig
	}
	return nil
}

func (f *fakeExecutor) writeRunner(dir string, configured bool) {
	if dir == "" {
		return
	}
	marker := filepath.Join(dir, ".runner")
	f.mu.Lock()
	defer f.mu.Unlock()
	if configured {
		_ = os.MkdirAll(dir, 0o755)
		_ = os.WriteFile(marker, []byte(`{"agent":{}}`), 0o644)
		return
	}
	_ = os.Remove(marker)
}

// svcAction performs a svc.sh subcommand (state transition and/or
// status query) and returns what the real script would print.
func (f *fakeExecutor) svcAction(args []string, dir string) (string, error) {
	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}
	f.mu.Lock()
	state := f.svcState
	f.mu.Unlock()
	switch sub {
	case "install":
		if f.FailSvcInstall != nil {
			return f.FailSvcInstall.Error(), f.FailSvcInstall
		}
		if state != "not-installed" {
			return "", errors.New("svc.sh: error: exists plist (service already installed)")
		}
		f.setSvcState("stopped")
		f.writePlist(true)
		if f.CrashSvcInstall != nil {
			return "", f.CrashSvcInstall
		}
		return "Creating launch runner\nsvc install complete\n", nil
	case "start":
		if f.FailSvcStart != nil {
			return f.FailSvcStart.Error(), f.FailSvcStart
		}
		if state == "not-installed" {
			return "", errors.New("svc.sh: service is not installed")
		}
		f.setSvcState("running")
		return "", nil
	case "stop":
		if f.FailSvcStop != nil {
			return f.FailSvcStop.Error(), f.FailSvcStop
		}
		if state == "not-installed" {
			return "", errors.New("svc.sh: service is not installed")
		}
		f.setSvcState("stopped")
		return "", nil
	case "uninstall":
		if f.FailSvcUninstall != nil {
			return f.FailSvcUninstall.Error(), f.FailSvcUninstall
		}
		f.setSvcState("not-installed")
		f.writePlist(false)
		return "", nil
	case "status":
		return f.svcStatusOutput(dir, args)
	default:
		return "", fmt.Errorf("fakeExecutor: unexpected svc.sh subcommand %q", sub)
	}
}

func (f *fakeExecutor) svcStatusOutput(dir string, args []string) (string, error) {
	if f.FailSvcStatus != nil {
		return "", f.FailSvcStatus
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	// Mirrors the real darwin svc.sh: always exits 0; prints "not
	// installed", "Stopped" or "Started:".
	switch f.svcState {
	case "running":
		return "status actions.runner.acme-widgets.host-runner01:\n\n/tmp/actions.runner.acme-widgets.host-runner01.plist\n\nStarted:\n12345\t0\tactions.runner.acme-widgets.host-runner01\n\n", nil
	case "stopped":
		return "status actions.runner.acme-widgets.host-runner01:\n\n/tmp/actions.runner.acme-widgets.host-runner01.plist\n\nStopped\n\n", nil
	default:
		return "status actions.runner.acme-widgets.host-runner01:\n\nnot installed\n\n", nil
	}
}

// scAction performs an sc.exe subcommand and returns its output.
func (f *fakeExecutor) scAction(args []string) (string, error) {
	sub := ""
	if len(args) > 0 {
		sub = strings.ToLower(args[0])
	}
	switch sub {
	case "query":
		return f.scQueryOutput(args)
	case "stop":
		if f.FailScStop != nil {
			return f.FailScStop.Error(), f.FailScStop
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.svcState == "not-installed" {
			return "", &fakeExitError{code: 1060, msg: "[SC] OpenService FAILED 1060"}
		}
		if f.WindowsStopPendingCount > 0 {
			f.stopPending = f.WindowsStopPendingCount
			f.WindowsStopPendingCount = 0
			return "[SC] SetServiceStatus: STOP_PENDING\n", nil
		}
		f.svcState = "stopped"
		return "", nil
	case "delete":
		if f.FailScDelete != nil {
			return f.FailScDelete.Error(), f.FailScDelete
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.svcState == "not-installed" {
			return "", &fakeExitError{code: 1060, msg: "[SC] OpenService FAILED 1060"}
		}
		f.svcState = "not-installed"
		return "", nil
	case "start":
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.svcState == "not-installed" {
			return "", &fakeExitError{code: 1060, msg: "[SC] OpenService FAILED 1060"}
		}
		f.svcState = "running"
		return "", nil
	default:
		return "", fmt.Errorf("fakeExecutor: unexpected sc.exe subcommand %q", sub)
	}
}

func (f *fakeExecutor) scQueryOutput(args []string) (string, error) {
	if f.FailScQuery != nil {
		return "", f.FailScQuery
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stopPending > 0 {
		f.stopPending--
		if f.stopPending == 0 {
			f.svcState = "stopped"
		}
		return "SERVICE_NAME: actions.runner.x-y.host-runner01\n        STATE              : 3  STOP_PENDING\n", nil
	}
	switch f.svcState {
	case "running":
		return "SERVICE_NAME: actions.runner.x-y.host-runner01\n        STATE              : 4  RUNNING\n", nil
	case "stopped":
		return "SERVICE_NAME: actions.runner.x-y.host-runner01\n        STATE              : 1  STOPPED\n", nil
	default:
		return "[SC] EnumQueryServicesStatus:OpenService FAILED 1060:\nThe specified service does not exist as an installed service.\n", &fakeExitError{code: 1060}
	}
}
