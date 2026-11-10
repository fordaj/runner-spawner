package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// runnerStatus is one line of `list` / `status` output.
type runnerStatus struct {
	Name         string `json:"name"`
	Dir          string `json:"dir"`
	Configured   bool   `json:"configured"`
	ServiceState string `json:"serviceState"` // running|stopped|not-installed|unknown|unsupported
	Display      string `json:"display"`
}

// statusOne inspects one runner dir without mutating anything.
func statusOne(cfg *Config, p platform, name string) (runnerStatus, error) {
	dir, err := runnerDirFor(cfg, name)
	if err != nil {
		return runnerStatus{}, err
	}
	st := runnerStatus{
		Name:    name,
		Dir:     dir,
		Display: displayName(name),
	}
	if !runnerExists(dir) {
		st.ServiceState = "not-installed"
		return st, nil
	}
	st.Configured = isConfigured(dir)
	if !p.Supported {
		st.ServiceState = "unsupported"
		return st, nil
	}
	st.ServiceState = queryServiceState(cfg, p, dir, name)
	return st, nil
}

// queryServiceState best-effort probes the service. It never returns an
// error: unknown probe results collapse to "unknown" so status stays usable.
func queryServiceState(cfg *Config, p platform, dir, name string) string {
	if p.IsWindows {
		out, err := runCmdCapture("sc.exe", "query", windowsServiceName(cfg, name))
		if err != nil {
			if strings.Contains(out, "does not exist") {
				return "not-installed"
			}
			return "unknown"
		}
		upper := strings.ToUpper(out)
		switch {
		case strings.Contains(upper, "RUNNING"):
			return "running"
		case strings.Contains(upper, "STOPPED"):
			return "stopped"
		default:
			return "unknown"
		}
	}
	// macOS svc.sh status prints e.g. "* service is running ..." style
	// output; probe it and match on "running" case-insensitively.
	svc := serviceScript(dir, p)
	if _, err := os.Stat(svc); err != nil {
		return "unknown"
	}
	out, _ := runCmdCapture(svc, "status")
	upper := strings.ToUpper(out)
	switch {
	case strings.Contains(upper, "RUNNING"):
		return "running"
	case strings.Contains(upper, "STOPPED"), strings.Contains(upper, "NOT"):
		// svc.sh says "not running" when stopped.
		if strings.Contains(upper, "STOPPED") {
			return "stopped"
		}
		return "stopped"
	default:
		return "unknown"
	}
}

// listNames returns the union of config names and on-disk runner dirs
// (sorted). On-disk detection is what makes bare `list` useful before any
// config exists.
func listNames(cfg *Config) ([]string, error) {
	seen := map[string]bool{}
	var names []string
	add := func(n string) {
		n = strings.TrimSpace(n)
		if n == "" || seen[n] {
			return
		}
		seen[n] = true
		names = append(names, n)
	}
	for _, n := range cfg.RunnerNames {
		add(n)
	}
	root, err := cfg.RunnersRoot()
	if err == nil {
		if entries, rerr := os.ReadDir(root); rerr == nil {
			for _, e := range entries {
				// The cache subdir is not a runner.
				if e.IsDir() && e.Name() != "cache" {
					add(e.Name())
				}
			}
		}
	}
	sortStrings(names)
	return names, nil
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// ---- create ----

func createOne(cfg *Config, p platform, name string, stdout, stderr io.Writer) error {
	logf := func(s string) {
		if !cfg.Quiet {
			fmt.Fprintln(stderr, "runner-spawner:", s)
		}
	}
	if err := p.requireSupported("create"); err != nil {
		return err
	}
	if err := adminCheck(p); err != nil {
		return err
	}
	dir, err := runnerDirFor(cfg, name)
	if err != nil {
		return err
	}
	labels := p.Labels(cfg, name)
	labelsJoined := strings.Join(labels, ",")

	if cfg.DryRun {
		fmt.Fprintf(stdout, "create %s -> %s\n", displayName(name), dir)
		fmt.Fprintf(stdout, "  url=%s group=%s labels=%s\n", cfg.runnerURL(), cfg.RunnerGroup, labelsJoined)
		if archiveURL, _, err := versionedAsset(cfg, p); err == nil {
			fmt.Fprintf(stdout, "  would fetch %s\n", archiveURL)
		}
		fmt.Fprintf(stdout, "  would run %s --unattended --labels %s --name %s --runnergroup %s --replace\n", configScript(dir, p), labelsJoined, displayName(name), cfg.RunnerGroup)
		if p.IsWindows {
			fmt.Fprintf(stdout, "  would configure Windows service as %q\n", cfg.WindowsUsername)
		}
		return nil
	}

	// Windows service account is required for --runasservice.
	if p.IsWindows {
		if cfg.WindowsUsername == "" {
			pwd, err := promptLine(stdinReader(), "Enter Windows service account username (the account the runner service runs as): ")
			if err != nil {
				return fmt.Errorf("--windows-username is required on Windows (set RUNNER_SPAWNER_WINDOWS_USERNAME or pass --windows-username): %w", err)
			}
			cfg.WindowsUsername = strings.TrimSpace(pwd)
		}
		if cfg.WindowsPassword == "" {
			pwd, err := promptPassword(stdinReader(), "Enter password for Windows service account: ")
			if err != nil {
				return fmt.Errorf("--windows-password is required on Windows (set RUNNER_SPAWNER_WINDOWS_PASSWORD or pass --windows-password): %w", err)
			}
			cfg.WindowsPassword = pwd
		}
	}

	tok, err := runnerToken(cfg)
	if err != nil {
		return err
	}
	logf(fmt.Sprintf("creating %s in %s", displayName(name), dir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	archivePath, err := ensureArchive(cfg, p, defaultHTTPClient(), logf)
	if err != nil {
		return err
	}
	if err := extractArchive(archivePath, dir); err != nil {
		return fmt.Errorf("extracting %s: %w", filepath.Base(archivePath), err)
	}

	// Configure.
	script := configScript(dir, p)
	args := []string{
		"--url", cfg.runnerURL(),
		"--token", tok,
		"--unattended",
		"--labels", labelsJoined,
		"--name", displayName(name),
		"--runnergroup", cfg.RunnerGroup,
		"--replace",
	}
	if p.IsWindows {
		args = append(args,
			"--runasservice",
			"--windowslogonaccount", cfg.WindowsUsername,
			"--windowslogonpassword", cfg.WindowsPassword,
		)
	}
	logf(fmt.Sprintf("configuring (labels=%s)", labelsJoined))
	if err := runScript(script, args...); err != nil {
		return fmt.Errorf("configuring runner %s: %w", name, err)
	}

	if !p.IsWindows {
		svc := serviceScript(dir, p)
		for _, sub := range []string{"install", "start"} {
			logf(fmt.Sprintf("svc.sh %s", sub))
			if err := runScriptInDir(svc, []string{sub}, dir); err != nil {
				return fmt.Errorf("runner %s: svc.sh %s: %w", name, sub, err)
			}
		}
	}
	logf(fmt.Sprintf("created %s", displayName(name)))
	return nil
}

// ---- remove ----

func removeOne(cfg *Config, p platform, name string, stderr io.Writer) error {
	logf := func(s string) {
		if !cfg.Quiet {
			fmt.Fprintln(stderr, "runner-spawner:", s)
		}
	}
	if err := p.requireSupported("remove"); err != nil {
		return err
	}
	if err := adminCheck(p); err != nil {
		return err
	}
	dir, err := runnerDirFor(cfg, name)
	if err != nil {
		return err
	}
	if !runnerExists(dir) {
		logf(fmt.Sprintf("runner %s not installed (no %s); nothing to remove", name, dir))
		return nil
	}

	if cfg.DryRun {
		fmt.Fprintf(os.Stdout, "remove %s -> %s\n", displayName(name), dir)
		fmt.Fprintf(os.Stdout, "  would stop+uninstall service, remove from GitHub, and delete the directory\n")
		return nil
	}

	tok, err := runnerToken(cfg)
	if err != nil {
		return err
	}
	logf(fmt.Sprintf("removing %s", displayName(name)))

	// Stop + uninstall the service first, tolerating "already stopped" /
	// "not installed" states. This is the fix for the bash bug where
	// removing a running runner failed partway through: the service must
	// be gone before `config.sh remove` (v2.336+ requires it on macOS).
	stopService(cfg, p, dir, name, logf)
	uninstallService(cfg, p, dir, name, logf)

	script := configScript(dir, p)
	if err := runScript(script, "remove", "--token", tok); err != nil {
		return fmt.Errorf("removing runner %s from GitHub: %w (service already stopped; re-run `remove` to retry, or delete %s manually)", name, err, dir)
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("deleting %s: %w", dir, err)
	}
	logf(fmt.Sprintf("removed %s", displayName(name)))
	return nil
}

// ---- start / stop ----

func startOne(cfg *Config, p platform, name string, stderr io.Writer) error {
	return controlOne(cfg, p, name, "start", stderr)
}

func stopOne(cfg *Config, p platform, name string, stderr io.Writer) error {
	return controlOne(cfg, p, name, "stop", stderr)
}

func controlOne(cfg *Config, p platform, name, verb string, stderr io.Writer) error {
	logf := func(s string) {
		if !cfg.Quiet {
			fmt.Fprintln(stderr, "runner-spawner:", s)
		}
	}
	if err := p.requireSupported(verb); err != nil {
		return err
	}
	if err := adminCheck(p); err != nil {
		return err
	}
	dir, err := runnerDirFor(cfg, name)
	if err != nil {
		return err
	}
	if !runnerExists(dir) {
		return fmt.Errorf("runner %s not installed (no %s)", name, dir)
	}

	want := "stopped"
	if verb == "start" {
		want = "running"
	}
	if st := queryServiceState(cfg, p, dir, name); st == want {
		logf(fmt.Sprintf("runner %s already %s", displayName(name), want))
		return nil
	}
	if cfg.DryRun {
		fmt.Fprintf(os.Stdout, "%s %s -> %s\n", verb, displayName(name), dir)
		return nil
	}
	logf(fmt.Sprintf("%s %s", verb, displayName(name)))
	if p.IsWindows {
		if err := runCmd("sc.exe", verb, windowsServiceName(cfg, name)); err != nil {
			return fmt.Errorf("runner %s: sc.exe %s: %w", name, verb, err)
		}
		return nil
	}
	svc := serviceScript(dir, p)
	sub := verb // svc.sh start/stop
	if err := runScriptInDir(svc, []string{sub}, dir); err != nil {
		return fmt.Errorf("runner %s: svc.sh %s: %w", name, sub, err)
	}
	return nil
}

func stopService(cfg *Config, p platform, dir, name string, logf func(string)) {
	if queryServiceState(cfg, p, dir, name) != "running" {
		return
	}
	logf(fmt.Sprintf("stopping %s", displayName(name)))
	if p.IsWindows {
		if _, err := runCmdCapture("sc.exe", "stop", windowsServiceName(cfg, name)); err != nil {
			logf(fmt.Sprintf("warning: sc.exe stop failed for %s: %v (continuing)", displayName(name), err))
		}
		return
	}
	svc := serviceScript(dir, p)
	if out, err := runScriptCapture(svc, "stop"); err != nil {
		logf(fmt.Sprintf("warning: svc.sh stop failed for %s: %v: %s (continuing)", displayName(name), err, strings.TrimSpace(out)))
	}
}

func uninstallService(cfg *Config, p platform, dir, name string, logf func(string)) {
	logf(fmt.Sprintf("uninstalling %s service", displayName(name)))
	if p.IsWindows {
		out, err := runCmdCapture("sc.exe", "delete", windowsServiceName(cfg, name))
		if err != nil && !strings.Contains(out, "does not exist") {
			logf(fmt.Sprintf("warning: sc.exe delete failed for %s: %v: %s (continuing)", displayName(name), err, strings.TrimSpace(out)))
		}
		return
	}
	svc := serviceScript(dir, p)
	if out, err := runScriptCapture(svc, "uninstall"); err != nil {
		logf(fmt.Sprintf("warning: svc.sh uninstall failed for %s: %v: %s (continuing)", displayName(name), err, strings.TrimSpace(out)))
	}
}

// stdinReader is a seam for interactive prompts (tests can swap it).
var stdinReader = func() *bufio.Reader { return bufio.NewReader(os.Stdin) }

func promptLine(r *bufio.Reader, label string) (string, error) {
	fmt.Fprint(os.Stderr, label)
	line, err := r.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func promptPassword(r *bufio.Reader, label string) (string, error) {
	fmt.Fprint(os.Stderr, label)
	line, err := r.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	pwd := strings.TrimRight(line, "\r\n")
	fmt.Fprintln(os.Stderr)
	return pwd, nil
}
