package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// initFile is the JSON written by `runner-spawner init`.
type initFile struct {
	Owner         string   `json:"owner"`
	Repo          string   `json:"repo"`
	RunnerNames   []string `json:"runnerNames"`
	RunnersDir    string   `json:"runnersDir"`
	RunnerGroup   string   `json:"runnerGroup"`
	RunnerVersion string   `json:"runnerVersion"`
}

// confirmOverwrite reports whether an existing .runner-spawner.json may be
// overwritten. A non-terminal caller refuses with exit code 2; in a terminal
// the user is asked "Overwrite? [y/N]" and confirmed only on an explicit yes.
// ok is false when the user declines (nothing to do).
func confirmOverwrite(stdin io.Reader, target string, stdout, stderr io.Writer) (ok bool, err error) {
	if !stdinIsTerminal(stdin) {
		fmt.Fprintf(stderr, "runner-spawner: %s already exists; not overwriting\n", target)
		return false, &codedError{code: 2, msg: "config file exists"}
	}
	fmt.Fprintf(stderr, "%s already exists. Overwrite? [y/N] ", target)
	reader := bufio.NewReader(stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer != "y" && answer != "yes" {
		fmt.Fprintln(stdout, "keeping existing file; nothing written")
		return false, nil
	}
	return true, nil
}

// runInit implements `runner-spawner init`, modeled on `formattington init`:
// it examines the git remote of the current directory and writes a tailored
// ./.runner-spawner.json. A committed .runner-spawner.json already present
// is never overwritten non-interactively; in a terminal it prompts
// "Overwrite? [y/N]" and only proceeds on an explicit yes.
func runInit(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	dir := "."
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h", "help":
			fmt.Fprintln(stdout, "Usage: runner-spawner init [--dir DIR]")
			fmt.Fprintln(stdout, "")
			fmt.Fprintln(stdout, "Write a .runner-spawner.json for the repository in DIR (default .),")
			fmt.Fprintln(stdout, "detecting owner/repo from its git remote. Refuses to overwrite an")
			fmt.Fprintln(stdout, "existing .runner-spawner.json unless confirmed interactively.")
			return nil
		case "--dir":
			if i+1 >= len(args) {
				return usageErrf("--dir requires a value")
			}
			dir = args[i+1]
			i++
		default:
			return usageErrf("unknown flag %q for init (expected: --dir DIR)", args[i])
		}
	}

	target := filepath.Join(dir, DefaultConfigFile)

	// Overwrite guard.
	if _, err := os.Stat(target); err == nil {
		overwrite, err := confirmOverwrite(stdin, target, stdout, stderr)
		if err != nil {
			return err
		}
		if !overwrite {
			return nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	// owner/repo come from the git remote; the config is useless without them.
	owner, repo := "", ""
	if detectedOwner, detectedRepo, ok := detectGitRemoteOwnerRepo(dir); ok {
		owner, repo = detectedOwner, detectedRepo
	} else {
		return fmt.Errorf("could not detect owner/repo from the git remote of %s (is it a git clone with an origin remote?); create %s manually instead", dir, DefaultConfigFile)
	}

	cfg := initFile{
		Owner:         owner,
		Repo:          repo,
		RunnerNames:   []string{"runner01"},
		RunnersDir:    ".runners",
		RunnerGroup:   "Default",
		RunnerVersion: DefaultConfig().RunnerVersion,
	}

	body, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')

	if err := os.WriteFile(target, body, 0o644); err != nil {
		return err
	}

	fmt.Fprintf(stdout, "Wrote %s for %s/%s with runner names %v\n", target, cfg.Owner, cfg.Repo, cfg.RunnerNames)
	fmt.Fprintln(stdout, "Review it, commit it, then run `runner-spawner create` from the repo root.")
	return nil
}

// stdinIsTerminal reports whether the reader is an interactive terminal.
// Tests pass strings.Reader (non-terminal) or a pty-backed file (terminal).
func stdinIsTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
