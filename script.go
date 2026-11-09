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

// runScriptCapture executes a script and returns combined output.
func runScriptCapture(script string, args ...string) (string, error) {
	cmd := exec.Command(script, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
