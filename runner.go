package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// runCmd runs name+args inheriting stdio. Used for sc.exe / net.
func runCmd(name string, args ...string) error {
	return commandExecution.Run(name, args, "")
}

// runCmdCapture runs name+args, returning combined output.
func runCmdCapture(name string, args ...string) (string, error) {
	return commandExecution.Output(name, args, "")
}

// runnerDirFor returns the install directory for a runner name.
func runnerDirFor(cfg *Config, name string) (string, error) {
	root, err := cfg.RunnersRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, name), nil
}

// displayName is the Actions display name: hostname-name (the bash scripts'
// $HOST-$RUNNER_NAME). The on-disk dir stays just the name.
func displayName(name string) string {
	return hostname() + "-" + name
}

// runnerExists reports whether the runner dir holds an installed runner.
func runnerExists(dir string) bool {
	fi, err := os.Stat(dir)
	return err == nil && fi.IsDir()
}

// isConfigured reports whether config.sh has completed (the .runner file).
func isConfigured(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".runner"))
	return err == nil
}

// isExtracted reports whether the pinned runner archive was unpacked well
// enough for config.sh/svc.sh to run. A directory without config.sh (or
// without svc.sh on Unix) is treated as empty/partial — the result of an
// interrupted or failed extraction — and must be repaired by create.
func isExtracted(dir string, p platform) bool {
	cfgScript := configScript(dir, p)
	fi, err := os.Stat(cfgScript)
	if err != nil || !fi.Mode().IsRegular() {
		return false
	}
	if !p.IsWindows {
		svc := serviceScript(dir, p)
		fi, err := os.Stat(svc)
		if err != nil || !fi.Mode().IsRegular() {
			return false
		}
	}
	return true
}

// configScript returns the absolute path to the runner's config script.
func configScript(dir string, p platform) string {
	return filepath.Join(dir, p.ConfigBin)
}

// serviceScript returns the absolute path to svc.sh.
func serviceScript(dir string, p platform) string {
	return filepath.Join(dir, p.ServiceBin)
}

// serviceName is the registration name for the runner's service: the
// Actions runner uses actions.runner.<owner>-<repo>.<display> as both the
// sc.exe service name (Windows) and the launchd label / plist basename
// (macOS).
func serviceName(cfg *Config, name string) string {
	return fmt.Sprintf("actions.runner.%s-%s.%s", cfg.Owner, cfg.Repo, displayName(name))
}

// macosServicePlist returns the user-level LaunchAgent plist path for the
// runner's service on macOS.
func macosServicePlist(homeDir string, cfg *Config, name string) string {
	return filepath.Join(homeDir, "Library", "LaunchAgents", serviceName(cfg, name)+".plist")
}

// userHomeDir is a seam so tests can redirect LaunchAgent lookups into a
// temp dir.
var userHomeDir = os.UserHomeDir
