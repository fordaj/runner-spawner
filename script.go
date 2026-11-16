package main

import (
	"os"
	"os/exec"
)

// commandExecutor spawns external commands. It is a seam for tests:
// unit tests swap commandExecution for a fake so svc.sh, config.sh /
// config.cmd, sc.exe and net can be faked on any OS (see
// lifecycle_test.go).
var commandExecution commandExecutor = realExecutor{}

type commandExecutor interface {
	// Run executes name+args streaming stdio to the process's stdout and
	// stderr. dir sets the working directory; "" inherits the caller's.
	Run(name string, args []string, dir string) error
	// Output executes name+args and returns combined stdout+stderr.
	Output(name string, args []string, dir string) (string, error)
}

type realExecutor struct{}

func (realExecutor) Run(name string, args []string, dir string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = dir
	return cmd.Run()
}

func (realExecutor) Output(name string, args []string, dir string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// exitCoder is implemented by *exec.ExitError; fakes in tests implement
// it as well so exit codes (e.g. sc.exe's 1060 "does not exist") can be
// asserted without spawning real processes.
type exitCoder interface {
	ExitCode() int
}

// runScriptInDir executes a shell script (config.sh, svc.sh) or a batch file
// (config.cmd) with dir as working directory.
func runScriptInDir(script string, args []string, dir string) error {
	return commandExecution.Run(script, args, dir)
}

// runScriptCaptureInDir executes a script from dir and returns combined output.
func runScriptCaptureInDir(script string, args []string, dir string) (string, error) {
	return commandExecution.Output(script, args, dir)
}
