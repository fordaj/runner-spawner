package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// runnerStatus is one line of `list` / `status` output.
type runnerStatus struct {
	Name         string `json:"name"`
	Dir          string `json:"dir"`
	Configured   bool   `json:"configured"`
	ServiceState string `json:"serviceState"` // running|stopped|pending|not-installed|unknown|unsupported
	Display      string `json:"display"`
	Hint         string `json:"hint,omitempty"` // remediation hint when not healthy
}

// statusOne inspects one runner dir without mutating anything.
func statusOne(cfg *Config, p platform, name string) (runnerStatus, error) {
	st := DetectLifecycle(cfg, p, name)
	rs := runnerStatus{
		Name:         name,
		Dir:          st.Dir,
		Configured:   st.Configured,
		ServiceState: st.ServiceState,
		Display:      displayName(name),
		Hint:         hintFor(p, st),
	}
	return rs, nil
}

// queryServiceState best-effort probes the service. It never returns an
// error: unknown probe results collapse to "unknown" so status stays usable.
//
// macOS: the LaunchAgent plist's existence defines "installed" (svc.sh
// status always exits 0, so it cannot be trusted for that); svc.sh
// status output ("Started:" / "Stopped") decides running vs stopped.
//
// Windows: sc.exe exit code 1060/1072 (does not exist / marked for
// deletion) means not installed; the numeric STATE code in `sc.exe query`
// output is locale-independent and decides the rest (see
// windowsServiceStateFromOutput).
func queryServiceState(cfg *Config, p platform, dir, name string) string {
	if p.IsWindows {
		out, err := runCmdCapture("sc.exe", "query", windowsServiceName(cfg, name))
		if err != nil {
			var ec exitCoder
			if errors.As(err, &ec) {
				// 1060 = service does not exist, 1072 = marked for
				// deletion (already gone enough for our purposes).
				if ec.ExitCode() == 1060 || ec.ExitCode() == 1072 {
					return "not-installed"
				}
			}
			// Fallback for fakes/errors that carry no code: the output
			// usually echoes the raw Win32 code.
			if strings.Contains(out, "1060") || strings.Contains(out, "1072") {
				return "not-installed"
			}
			return "unknown"
		}
		// Locale-independent: numeric STATE code wins; the English
		// substring match in windowsServiceStateFromOutput is a
		// documented last resort.
		if st := windowsServiceStateFromOutput(out); st != "" {
			return st
		}
		return "unknown"
	}
	// macOS. The plist's existence defines "installed".
	home, homeErr := userHomeDir()
	plistExists := false
	if homeErr == nil {
		if _, err := os.Stat(macosServicePlist(home, cfg, name)); err == nil {
			plistExists = true
		}
	}
	if !plistExists {
		return "not-installed"
	}
	svc := serviceScript(dir, p)
	_, svcErr := os.Stat(svc)
	if svcErr != nil {
		// Plist exists but the dir is gone: orphan waiting for cleanup.
		return "unknown"
	}
	out, err := runScriptCaptureInDir(svc, []string{"status"}, dir)
	upper := strings.ToUpper(out)
	switch {
	case strings.Contains(upper, "STARTED"):
		return "running"
	case strings.Contains(upper, "STOPPED"):
		return "stopped"
	case strings.Contains(upper, "NOT INSTALLED"):
		return "not-installed"
	default:
		if err != nil {
			return "unknown"
		}
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
				// The cache and locks subdirs are not runners.
				if e.IsDir() && e.Name() != "cache" && e.Name() != ".locks" {
					add(e.Name())
				}
			}
		}
	}
	sort.Strings(names)
	return names, nil
}

// ---- create ----

// configPreserveFiles are copied from an existing install into the staging
// dir on re-extract so config state survives a version swap.
var configPreserveFiles = []string{".runner", ".credentials", ".credentials_rsaparams"}

func createOne(cfg *Config, p platform, name string, stdout, stderr io.Writer) error {
	logf := func(s string) {
		if !cfg.Quiet {
			fmt.Fprintln(stderr, "runner-spawner:", s)
		}
	}
	warnf := func(s string) { fmt.Fprintln(stderr, "runner-spawner: warning:", s) }
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
	root, err := cfg.RunnersRoot()
	if err != nil {
		return err
	}
	labels := p.Labels(cfg, name)
	labelsJoined := strings.Join(labels, ",")

	unlock, err := lockRunner(root, name)
	if err != nil {
		return err
	}
	defer unlock()

	st := DetectLifecycle(cfg, p, name)

	// Idempotency: an already-running, configured runner is healthy —
	// don't reconfigure a live runner.
	if st.Configured && st.ServiceState == "running" {
		if cfg.DryRun {
			fmt.Fprintf(stdout, "create %s -> %s\n  already running; nothing to do\n", displayName(name), dir)
			return nil
		}
		logf(fmt.Sprintf("runner %s already running; nothing to do", displayName(name)))
		return nil
	}

	if cfg.DryRun {
		fmt.Fprintf(stdout, "create %s -> %s\n", displayName(name), dir)
		fmt.Fprintf(stdout, "  url=%s group=%s labels=%s\n", cfg.runnerURL(), cfg.RunnerGroup, labelsJoined)
		if archiveURL, _, err := versionedAsset(cfg, p); err == nil {
			fmt.Fprintf(stdout, "  would fetch %s\n", archiveURL)
		}
		switch {
		case !st.Extracted, st.MarkerVersion != strings.TrimPrefix(cfg.RunnerVersion, "v"):
			fmt.Fprintf(stdout, "  would extract (atomically) into %s\n", dir)
		default:
			fmt.Fprintf(stdout, "  existing extraction kept (version marker matches)\n")
		}
		if !st.Configured {
			fmt.Fprintf(stdout, "  would run %s --unattended --labels %s --name %s --runnergroup %s --replace\n", configScript(dir, p), labelsJoined, displayName(name), cfg.RunnerGroup)
		} else {
			fmt.Fprintf(stdout, "  runner already configured; would skip config.sh\n")
		}
		if p.IsWindows {
			fmt.Fprintf(stdout, "  would ensure Windows service is installed and started\n")
		} else {
			fmt.Fprintf(stdout, "  would ensure service is installed and started (svc.sh)\n")
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

	logf(fmt.Sprintf("creating %s in %s", displayName(name), dir))

	// 1) Extraction (atomic). Skipped when the installed files match the
	// pinned version — this also heals empty/partial dirs and strips
	// stale files from older versions.
	wantVersion := strings.TrimPrefix(cfg.RunnerVersion, "v")
	if !st.Extracted || st.MarkerVersion != wantVersion {
		archivePath, err := ensureArchive(cfg, p, defaultHTTPClient(), logf)
		if err != nil {
			return err
		}
		if err := extractArchiveAtomic(archivePath, dir, configPreserveFiles, cfg.RunnerVersion); err != nil {
			return err
		}
		st = DetectLifecycle(cfg, p, name)
	}

	// 2) Configuration. Skipped when the runner is already configured.
	if !st.Configured {
		tok, err := runnerToken(cfg)
		if err != nil {
			return err
		}
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
		// config.sh/config.cmd must run with the runner dir as cwd.
		if err := runScriptInDir(script, args, dir); err != nil {
			return fmt.Errorf("configuring runner %s: %w — re-run `runner-spawner create --name %s` to resume", name, err, name)
		}
		if !isConfigured(dir) {
			return fmt.Errorf("configuring runner %s: config script exited cleanly but %s/.runner is missing — re-run `runner-spawner create --name %s` to retry", name, dir, name)
		}
	} else {
		logf("runner already configured; skipping config.sh")
	}

	// 3) Service installation & start.
	if p.IsWindows {
		if err := ensureWindowsService(cfg, p, dir, name, logf, warnf); err != nil {
			return err
		}
	} else {
		if err := ensureUnixService(cfg, p, dir, name, logf); err != nil {
			return err
		}
	}

	// 4) Post-condition verification.
	if !isConfigured(dir) {
		return fmt.Errorf("post-check failed: %s/.runner missing after create", dir)
	}
	if st := queryServiceState(cfg, p, dir, name); st != "running" {
		return fmt.Errorf("post-check failed: service for %s is %q after create — run `runner-spawner start --name %s` and check its logs", displayName(name), st, name)
	}
	logf(fmt.Sprintf("created %s", displayName(name)))
	return nil
}

// ensureUnixService installs (if needed) and starts the svc.sh service,
// tolerates already-installed states, and waits for the service to run.
func ensureUnixService(cfg *Config, p platform, dir, name string, logf func(string)) error {
	svc := serviceScript(dir, p)
	if _, err := os.Stat(svc); err != nil {
		return fmt.Errorf("svc.sh not found in %s — the extraction is incomplete; re-run `runner-spawner create --name %s`", dir, name)
	}
	st := queryServiceState(cfg, p, dir, name)
	if st == "not-installed" {
		logf("svc.sh install")
		if err := runScriptInDir(svc, []string{"install"}, dir); err != nil {
			// svc.sh is not idempotent against a partial/existing
			// install; re-probe and accept "installed now" as success.
			st2 := queryServiceState(cfg, p, dir, name)
			if st2 == "not-installed" {
				return fmt.Errorf("runner %s: svc.sh install: %w — re-run `runner-spawner create --name %s` to resume", name, err, name)
			}
			logf("svc.sh install reported an error but the service is now installed; continuing")
		}
		st = queryServiceState(cfg, p, dir, name)
	}
	if st != "running" {
		logf("svc.sh start")
		if err := runScriptInDir(svc, []string{"start"}, dir); err != nil {
			return fmt.Errorf("runner %s: svc.sh start: %w — re-run `runner-spawner start --name %s`", name, err, name)
		}
		ok := waitFor(serviceTimeout, servicePollInterval, func() bool {
			return queryServiceState(cfg, p, dir, name) == "running"
		})
		if !ok {
			return fmt.Errorf("runner %s: service did not report running within %s — check `svc.sh status` in %s", displayName(name), serviceTimeout, dir)
		}
	}
	return nil
}

// ensureWindowsService makes sure the Windows service (installed by
// config.cmd --runasservice) exists, is started, and reports running.
func ensureWindowsService(cfg *Config, p platform, dir, name string, logf func(string), warnf func(string)) error {
	svcName := windowsServiceName(cfg, name)
	st := queryServiceState(cfg, p, dir, name)
	switch st {
	case "not-installed":
		return fmt.Errorf("runner %s: config.cmd --runasservice did not install service %q — check %s/_diag logs; re-run `runner-spawner create --name %s` to retry", name, svcName, dir, name)
	case "stopped", "unknown", "pending":
		logf(fmt.Sprintf("starting service %s", svcName))
		if err := runCmd("sc.exe", "start", svcName); err != nil {
			// 1056 = service already running.
			warnf(fmt.Sprintf("sc.exe start for %s: %v (continuing)", svcName, err))
		}
	}
	ok := waitFor(serviceTimeout, servicePollInterval, func() bool {
		return queryServiceState(cfg, p, dir, name) == "running"
	})
	if !ok {
		return fmt.Errorf("runner %s: service %q did not report running within %s — check `sc.exe query %s`", displayName(name), svcName, serviceTimeout, svcName)
	}
	return nil
}

// ---- remove ----

func removeOne(cfg *Config, p platform, name string, stdout, stderr io.Writer) error {
	logf := func(s string) {
		if !cfg.Quiet {
			fmt.Fprintln(stderr, "runner-spawner:", s)
		}
	}
	warnf := func(s string) { fmt.Fprintln(stderr, "runner-spawner: warning:", s) }
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
	root, err := cfg.RunnersRoot()
	if err != nil {
		return err
	}

	unlock, err := lockRunner(root, name)
	if err != nil {
		return err
	}
	defer unlock()

	st := DetectLifecycle(cfg, p, name)

	if cfg.DryRun {
		fmt.Fprintf(stdout, "remove %s -> %s\n", displayName(name), dir)
		fmt.Fprintf(stdout, "  would stop+uninstall service (waiting for it to stop), deregister from GitHub (unless --local-only), and delete the directory\n")
		if cfg.LocalOnly {
			fmt.Fprintf(stdout, "  --local-only: GitHub deregistration would be skipped\n")
		}
		return nil
	}
	logf(fmt.Sprintf("removing %s", displayName(name)))

	// 1) Stop + uninstall the service first (macOS's config.sh requires
	// a stopped service since v2.336). Wait for the stop to settle
	// instead of fire-and-forget.
	dirOK := runnerExists(dir)
	if st.DirGone && st.ServiceState == "not-installed" {
		// Nothing local — but the GitHub registration may still exist
		// (e.g. a previous `remove` failed deregistration). Don't
		// early-return: the deregistration below completes the job.
		logf(fmt.Sprintf("runner %s not installed locally (no %s); checking GitHub for a lingering registration", name, dir))
	}
	svcPresent := p.IsWindows || (dirOK && fileExists(serviceScript(dir, p)))
	if dirOK && svcPresent {
		if err := stopService(cfg, p, dir, name, logf, warnf); err != nil {
			return err
		}
		if err := uninstallService(cfg, p, dir, name, logf, warnf); err != nil {
			return err
		}
		// Post-condition: the service is actually gone. If the runner
		// left an orphan (service survived the uninstall), try a
		// best-effort cleanup and then fail loudly if it persists.
		if !waitFor(serviceTimeout, servicePollInterval, func() bool {
			return queryServiceState(cfg, p, dir, name) == "not-installed"
		}) {
			warnf(fmt.Sprintf("service for %s is still registered after uninstall; attempting orphan cleanup", displayName(name)))
			cleanupOrphanService(cfg, p, dir, name, warnf)
			if !waitFor(serviceTimeout, servicePollInterval, func() bool {
				return queryServiceState(cfg, p, dir, name) == "not-installed"
			}) {
				return fmt.Errorf("service for %s is still installed after uninstall; remove it manually (%s) and re-run `runner-spawner remove --name %s`", displayName(name), manualServiceRemoveHint(cfg, p, name), name)
			}
		}
	} else if dirOK {
		// Partial/failed extraction on macOS: svc.sh is unusable, but a
		// LaunchAgent plist may have been created by a half-finished
		// install.
		warnf(fmt.Sprintf("runner dir %s is incomplete (no usable svc.sh); cleaning up any lingering service", dir))
		cleanupOrphanService(cfg, p, dir, name, warnf)
	} else if p.IsWindows && st.ServiceState != "not-installed" {
		// Dir gone/partial but sc.exe still sees the service: orphan.
		warnf(fmt.Sprintf("runner dir %s is missing/incomplete but the Windows service for %s still exists; deleting it", dir, displayName(name)))
		if err := stopService(cfg, p, dir, name, logf, warnf); err != nil {
			warnf(fmt.Sprintf("stopping orphaned service: %v (continuing)", err))
		}
		out, err := runCmdCapture("sc.exe", "delete", windowsServiceName(cfg, name))
		if err != nil {
			var ec exitCoder
			if !errors.As(err, &ec) || (ec.ExitCode() != 1060 && ec.ExitCode() != 1072) {
				warnf(fmt.Sprintf("sc.exe delete %s failed: %v: %s (continuing)", windowsServiceName(cfg, name), err, strings.TrimSpace(out)))
			}
		}
	} else if !p.IsWindows && st.ServiceState == "unknown" {
		// Dir gone but a LaunchAgent plist remains: orphan.
		warnf(fmt.Sprintf("runner dir %s is missing but a LaunchAgent plist remains; cleaning it up", dir))
		cleanupOrphanService(cfg, p, dir, name, warnf)
	}

	// 2) Remote deregistration. Never fetch the token before we know we
	// can use it; never strand the local removal on a token failure.
	deregistered := false
	deferNote := ""
	switch {
	case cfg.LocalOnly:
		warnf(fmt.Sprintf("--local-only set: skipping GitHub deregistration of %s; if it still exists on GitHub, delete it at https://github.com/%s/%s/settings/actions/runners", displayName(name), cfg.Owner, cfg.Repo))
		deferNote = "local-only removal: the runner may still be registered on GitHub"
	default:
		tok, tokErr := runnerToken(cfg)
		if tokErr != nil {
			warnf(fmt.Sprintf("cannot fetch a registration token (%v); skipping GitHub deregistration — local removal continues. Delete the runner manually at https://github.com/%s/%s/settings/actions/runners (or re-run with --local-only)", tokErr, cfg.Owner, cfg.Repo))
			deferNote = "token unavailable: the runner may still be registered on GitHub"
		} else {
			ok, note := deregisterRemote(cfg, p, dir, name, tok, st)
			deregistered = ok
			deferNote = note
		}
	}

	// 3) Delete the directory.
	if runnerExists(dir) {
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("deleting %s: %w (service stopped and deregistered; re-run `runner-spawner remove --name %s` to finish)", dir, err, name)
		}
	}
	if runnerExists(dir) {
		return fmt.Errorf("post-check failed: %s still exists after remove", dir)
	}
	if deregistered {
		logf(fmt.Sprintf("removed %s", displayName(name)))
	} else {
		logf(fmt.Sprintf("removed %s (local only)", displayName(name)))
		if deferNote != "" {
			warnf(deferNote)
		}
	}
	return nil
}

// deregisterRemote removes the runner from GitHub, either via the local
// config.sh (when the runner is configured) or via the admin API (when
// the dir is missing/partial). It reports whether deregistration is known
// to be done and a human-readable note. It never throws: failures are
// reported as a note so the local removal can proceed.
func deregisterRemote(cfg *Config, p platform, dir, name, tok string, st lifecycleState) (bool, string) {
	display := displayName(name)
	script := configScript(dir, p)
	if st.Configured && fileExists(script) {
		out, err := runScriptCaptureInDir(script, []string{"remove", "--token", tok}, dir)
		if err == nil {
			return true, ""
		}
		// A runner that GitHub no longer knows about is fine.
		lower := strings.ToLower(out)
		if strings.Contains(lower, "404") || strings.Contains(lower, "410") || strings.Contains(lower, "not found") {
			return true, "GitHub reports the runner is already deregistered (404); nothing remains remotely"
		}
		return false, fmt.Sprintf("GitHub deregistration failed (service stopped+uninstalled; re-run `runner-spawner remove --name %s` to retry): %v: %s", name, err, strings.TrimSpace(out))
	}
	// No local config to remove with: try the admin API.
	idOut, err := commandExecution.Output("gh", []string{
		"api", fmt.Sprintf("repos/%s/%s/actions/runners?per_page=100", cfg.Owner, cfg.Repo),
		"--jq", fmt.Sprintf(`.runners[] | select(.name == %q) | .id`, display),
	}, "")
	if err != nil {
		return false, fmt.Sprintf("could not list GitHub runners to deregister %s (%v); delete it manually at https://github.com/%s/%s/settings/actions/runners", display, err, cfg.Owner, cfg.Repo)
	}
	id := strings.TrimSpace(strings.Split(strings.TrimSpace(idOut), "\n")[0])
	if id == "" {
		return true, "no matching runner registration found on GitHub; nothing remains remotely"
	}
	if _, err := commandExecution.Output("gh", []string{
		"api", "-X", "DELETE", fmt.Sprintf("repos/%s/%s/actions/runners/%s", cfg.Owner, cfg.Repo, id),
	}, ""); err != nil {
		return false, fmt.Sprintf("could not delete GitHub runner %s (id %s): %v; delete it manually at https://github.com/%s/%s/settings/actions/runners", display, id, err, cfg.Owner, cfg.Repo)
	}
	return true, ""
}

// cleanupOrphanService removes a service that lingers after its runner dir
// is gone or broken. Best effort: warns and returns on any failure.
func cleanupOrphanService(cfg *Config, p platform, dir, name string, warnf func(string)) {
	if p.IsWindows {
		svcName := windowsServiceName(cfg, name)
		out, err := runCmdCapture("sc.exe", "delete", svcName)
		if err != nil {
			ec, ok := err.(exitCoder)
			if !ok || (ec.ExitCode() != 1060 && ec.ExitCode() != 1072) {
				warnf(fmt.Sprintf("sc.exe delete %s failed: %v: %s", svcName, err, strings.TrimSpace(out)))
			}
		}
		return
	}
	home, err := userHomeDir()
	if err != nil {
		warnf(fmt.Sprintf("cannot determine home dir to clean LaunchAgent plist: %v", err))
		return
	}
	plist := macosServicePlist(home, cfg, name)
	if _, err := os.Stat(plist); err == nil {
		if err := commandExecution.Run("launchctl", []string{"unload", "-w", plist}, ""); err != nil {
			warnf(fmt.Sprintf("launchctl unload %s failed: %v (continuing)", plist, err))
		}
		if err := os.Remove(plist); err != nil && !os.IsNotExist(err) {
			warnf(fmt.Sprintf("removing %s failed: %v", plist, err))
		}
	}
}

func manualServiceRemoveHint(cfg *Config, p platform, name string) string {
	if p.IsWindows {
		return fmt.Sprintf("sc.exe delete %s", windowsServiceName(cfg, name))
	}
	return fmt.Sprintf("launchctl remove %s", macosServiceLabel(cfg, name))
}

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular()
}

// ---- start / stop ----

func startOne(cfg *Config, p platform, name string, stdout, stderr io.Writer) error {
	return controlOne(cfg, p, name, "start", stdout, stderr)
}

func stopOne(cfg *Config, p platform, name string, stdout, stderr io.Writer) error {
	return controlOne(cfg, p, name, "stop", stdout, stderr)
}

func controlOne(cfg *Config, p platform, name, verb string, stdout, stderr io.Writer) error {
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
	root, err := cfg.RunnersRoot()
	if err != nil {
		return err
	}

	unlock, err := lockRunner(root, name)
	if err != nil {
		return err
	}
	defer unlock()

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
		fmt.Fprintf(stdout, "%s %s -> %s\n", verb, displayName(name), dir)
		return nil
	}
	logf(fmt.Sprintf("%s %s", verb, displayName(name)))
	if p.IsWindows {
		if err := runCmd("sc.exe", verb, windowsServiceName(cfg, name)); err != nil {
			return fmt.Errorf("runner %s: sc.exe %s: %w", name, verb, err)
		}
	} else {
		svc := serviceScript(dir, p)
		sub := verb // svc.sh start/stop
		if err := runScriptInDir(svc, []string{sub}, dir); err != nil {
			return fmt.Errorf("runner %s: svc.sh %s: %w", name, sub, err)
		}
	}
	ok := waitFor(serviceTimeout, servicePollInterval, func() bool {
		return queryServiceState(cfg, p, dir, name) == want
	})
	if !ok {
		return fmt.Errorf("runner %s: service did not report %q within %s — check the service manually", displayName(name), want, serviceTimeout)
	}
	return nil
}

// stopService stops the runner's service and waits (bounded) until the
// service reports stopped/not-installed. Command failure is a warning,
// not an abort — but a stop that never settles is an error, because
// macOS's config.sh remove requires a stopped service.
func stopService(cfg *Config, p platform, dir, name string, logf func(string), warnf func(string)) error {
	st := queryServiceState(cfg, p, dir, name)
	if st != "running" && st != "pending" {
		logf(fmt.Sprintf("service for %s is %s; nothing to stop", displayName(name), st))
		return nil
	}
	logf(fmt.Sprintf("stopping %s", displayName(name)))
	if p.IsWindows {
		svcName := windowsServiceName(cfg, name)
		if _, err := runCmdCapture("sc.exe", "stop", svcName); err != nil {
			out, _ := runCmdCapture("sc.exe", "query", svcName) // best-effort context
			_ = out
			warnf(fmt.Sprintf("sc.exe stop failed for %s: %v (waiting for the service to settle)", svcName, err))
		}
	} else {
		svc := serviceScript(dir, p)
		if out, err := runScriptCaptureInDir(svc, []string{"stop"}, dir); err != nil {
			warnf(fmt.Sprintf("svc.sh stop failed for %s: %v: %s (waiting for the service to settle)", displayName(name), err, strings.TrimSpace(out)))
		}
	}
	ok := waitFor(serviceTimeout, servicePollInterval, func() bool {
		st := queryServiceState(cfg, p, dir, name)
		return st == "stopped" || st == "not-installed"
	})
	if !ok {
		if p.IsWindows {
			return fmt.Errorf("service %s did not stop within %s — stop it manually (`sc.exe stop %s`) and re-run `runner-spawner remove --name %s`", displayName(name), serviceTimeout, windowsServiceName(cfg, name), name)
		}
		return fmt.Errorf("service %s did not stop within %s — stop it manually (`%s/svc.sh stop`) and re-run `runner-spawner remove --name %s`", displayName(name), serviceTimeout, dir, name)
	}
	return nil
}

// uninstallService uninstalls the service. Failure is a warning (the
// service might not be installed), but callers verify the post-condition
// and treat a persistent install as an error.
func uninstallService(cfg *Config, p platform, dir, name string, logf func(string), warnf func(string)) error {
	logf(fmt.Sprintf("uninstalling %s service", displayName(name)))
	if p.IsWindows {
		svcName := windowsServiceName(cfg, name)
		out, err := runCmdCapture("sc.exe", "delete", svcName)
		if err != nil {
			var ec exitCoder
			if !errors.As(err, &ec) || (ec.ExitCode() != 1060 && ec.ExitCode() != 1072) {
				warnf(fmt.Sprintf("sc.exe delete failed for %s: %v: %s (continuing)", svcName, err, strings.TrimSpace(out)))
			}
		}
		return nil
	}
	svc := serviceScript(dir, p)
	if _, err := os.Stat(svc); err != nil {
		warnf(fmt.Sprintf("svc.sh not found in %s; cannot uninstall service (continuing)", dir))
		return nil
	}
	if out, err := runScriptCaptureInDir(svc, []string{"uninstall"}, dir); err != nil {
		warnf(fmt.Sprintf("svc.sh uninstall failed for %s: %v: %s (continuing)", displayName(name), err, strings.TrimSpace(out)))
	}
	return nil
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
