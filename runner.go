package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// runCmd runs name+args inheriting stdio except that stdout is captured when
// the caller needs it (see runCmdCapture). Used for svc.sh / sc.exe / net.
func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runCmdCapture runs name+args, returning combined output.
func runCmdCapture(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
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

// configScript returns the absolute path to the runner's config script.
func configScript(dir string, p platform) string {
	return filepath.Join(dir, p.ConfigBin)
}

// serviceScript returns the absolute path to svc.sh.
func serviceScript(dir string, p platform) string {
	return filepath.Join(dir, p.ServiceBin)
}

// windowsServiceName matches actions runner + sc.exe usage:
// actions.runner.<owner>-<repo>.<display>
func windowsServiceName(cfg *Config, name string) string {
	return fmt.Sprintf("actions.runner.%s-%s.%s", cfg.Owner, cfg.Repo, displayName(name))
}
