package main

import (
	"regexp"
	"strings"
	"time"
)

// lifecycleState is the single source of truth for where a runner stands.
// It is derived from disk (dir, .runner, version marker) plus a service
// probe — never from a journal file — so any interrupted operation can be
// resumed by simply re-running the same command.
type lifecycleState struct {
	Name          string
	Dir           string
	DirGone       bool
	Extracted     bool
	Configured    bool
	MarkerVersion string
	ServiceState  string // running|stopped|pending|not-installed|unknown|unsupported
}

// DetectLifecycle inspects one runner without mutating anything.
func DetectLifecycle(cfg *Config, p platform, name string) lifecycleState {
	dir, err := runnerDirFor(cfg, name)
	st := lifecycleState{Name: name, Dir: dir}
	if err == nil {
		st.DirGone = !runnerExists(dir)
		st.Extracted = isExtracted(dir, p)
		st.Configured = isConfigured(dir)
		st.MarkerVersion = installedRunnerVersion(dir)
	} else {
		st.DirGone = true
	}
	if p.Supported {
		// sc.exe can see a service whose dir is gone; the darwin probe
		// falls back to the LaunchAgent plist check in the same way.
		st.ServiceState = queryServiceState(cfg, p, dir, name)
	} else {
		st.ServiceState = "unsupported"
	}
	return st
}

// hintFor returns a remediation hint for the observed state, or "" when
// the runner looks healthy (or absent entirely).
func hintFor(p platform, st lifecycleState) string {
	switch {
	case st.DirGone:
		if st.ServiceState == "unknown" || st.ServiceState == "stopped" || st.ServiceState == "running" {
			return "runner dir is missing but the service appears to be installed — run `runner-spawner remove --name <name>` to clean up"
		}
		return "not installed — run `runner-spawner create --name <name>` to create"
	case !st.Extracted:
		return "incomplete installation (extraction never finished) — run `runner-spawner create --name <name>` to repair"
	case !st.Configured:
		return "extracted but not configured — run `runner-spawner create --name <name>`"
	case st.ServiceState == "not-installed":
		return "configured but service not installed — run `runner-spawner create --name <name>`"
	case st.ServiceState == "stopped":
		return "stopped — run `runner-spawner start --name <name>` or `runner-spawner remove --name <name>`"
	case st.ServiceState == "pending":
		return "service is starting/stopping — wait and re-check with `runner-spawner status --name <name>`"
	case st.ServiceState == "unknown":
		return "service state unknown — check manually, then run `runner-spawner create --name <name>`"
	default:
		return ""
	}
}

// Poll timeouts for service transitions. Overridable in tests.
var (
	serviceTimeout      = 60 * time.Second
	servicePollInterval = 500 * time.Millisecond
)

// waitFor polls fn every interval until it returns true or the timeout
// elapses.
func waitFor(timeout, interval time.Duration, fn func() bool) bool {
	deadline := time.Now().Add(timeout)
	for {
		if fn() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(interval)
	}
}

// windowsStateRe finds the numeric STATE code in `sc.exe query` output.
// Codes: 1 STOPPED, 2 START_PENDING, 3 STOP_PENDING, 4 RUNNING,
// 5 CONTINUE_PENDING, 6 PAUSE_PENDING, 7 PAUSED (the English words are
// localized on non-English Windows, so only the number is trustworthy).
var windowsStateRe = regexp.MustCompile(`:\s*([1-7])\b`)

// windowsServiceStateFromOutput parses `sc.exe query` output using the
// locale-independent numeric STATE code (1=STOPPED ... 4=RUNNING ...).
// The textual labels sc.exe prints are localized on non-English Windows,
// so the number is the only reliable signal. Returns "" when no numeric
// state is found.
func windowsServiceStateFromOutput(out string) string {
	for _, line := range strings.Split(out, "\n") {
		m := windowsStateRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		switch m[1] {
		case "1":
			return "stopped"
		case "4":
			return "running"
		case "2", "3", "5", "6":
			return "pending"
		default:
			return "unknown"
		}
	}
	// Documented English fallback, kept only for outputs where the numeric
	// state cannot be located (unexpected format).
	upper := strings.ToUpper(out)
	switch {
	case strings.Contains(upper, "RUNNING"):
		return "running"
	case strings.Contains(upper, "STOPPED"):
		return "stopped"
	default:
		return ""
	}
}
