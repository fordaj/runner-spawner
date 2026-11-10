package main

import (
	"os"
	"os/exec"
)

// runScript executes a shell script (config.sh, svc.sh) or a batch file
// (config.cmd) inheriting stdio so the Actions runner output streams.
func runScript(script string, args ...string) error {
	cmd := exec.Command(script, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runScriptInDir executes a shell script with the given working directory.
// If dir is empty, the current working directory is used (backward compatible).
func runScriptInDir(script string, args []string, dir string) error {
	cmd := exec.Command(script, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = dir
	return cmd.Run()
}

// runScriptCapture executes a script and returns combined output.
func runScriptCapture(script string, args ...string) (string, error) {
	cmd := exec.Command(script, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
