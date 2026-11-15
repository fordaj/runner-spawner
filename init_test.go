package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initTestRepo creates a temporary git clone whose origin points at rawURL.
func initTestRepo(t *testing.T, rawURL string) string {
	t.Helper()
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init")
	git("remote", "add", "origin", rawURL)
	return dir
}

func readInitFile(t *testing.T, dir string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, DefaultConfigFile))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	return parsed
}

func TestInitWritesConfigFromRemote(t *testing.T) {
	dir := initTestRepo(t, "git@github.com:fordaj/example-repo.git")
	var stdout, stderr bytes.Buffer
	err := run([]string{"init", "--dir", dir}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("init: %v\nstderr: %s", err, stderr.String())
	}
	cfg := readInitFile(t, dir)
	if cfg["owner"] != "fordaj" || cfg["repo"] != "example-repo" {
		t.Errorf("owner/repo = %v/%v, want fordaj/example-repo", cfg["owner"], cfg["repo"])
	}
	names, ok := cfg["runnerNames"].([]any)
	if !ok || len(names) != 1 || names[0] != "runner01" {
		t.Errorf("runnerNames = %v, want [runner01]", cfg["runnerNames"])
	}
	if cfg["runnersDir"] != ".runners" {
		t.Errorf("runnersDir = %v, want .runners", cfg["runnersDir"])
	}
	if cfg["runnerGroup"] != "Default" {
		t.Errorf("runnerGroup = %v, want Default", cfg["runnerGroup"])
	}
	if cfg["runnerVersion"] != DefaultConfig().RunnerVersion {
		t.Errorf("runnerVersion = %v, want %v", cfg["runnerVersion"], DefaultConfig().RunnerVersion)
	}
}

func TestInitErrorsWithoutGitRemote(t *testing.T) {
	dir := t.TempDir() // not a git clone at all
	var stdout, stderr bytes.Buffer
	err := run([]string{"init", "--dir", dir}, strings.NewReader(""), &stdout, &stderr)
	if err == nil {
		t.Fatal("init without a git remote should fail")
	}
	if _, statErr := os.Stat(filepath.Join(dir, DefaultConfigFile)); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("no config should be written on failure, stat err = %v", statErr)
	}
}

func TestInitRefusesOverwriteWhenNonInteractive(t *testing.T) {
	dir := initTestRepo(t, "https://github.com/fordaj/example-repo.git")
	existing := `{"owner":"fordaj","repo":"example-repo","runnerNames":["runner02"],"runnerGroup":"Default"}`
	if err := os.WriteFile(filepath.Join(dir, DefaultConfigFile), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err := run([]string{"init", "--dir", dir}, strings.NewReader(""), &stdout, &stderr)
	var ce *codedError
	if !errors.As(err, &ce) || ce.code != 2 {
		t.Fatalf("init over existing file: err = %v, want exit code 2", err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, DefaultConfigFile))
	if string(data) != existing {
		t.Error("existing config was overwritten non-interactively")
	}
}

func TestInitInitConfigIsLoadable(t *testing.T) {
	// The file init writes must round-trip through LoadFile (unknown-key
	// rejection, type checks) so `init` can never emit an unreadable config.
	dir := initTestRepo(t, "ssh://git@github.com/fordaj/example-repo.git")
	var stdout, stderr bytes.Buffer
	if err := run([]string{"init", "--dir", dir}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("init: %v", err)
	}
	cfg := DefaultConfig()
	if err := cfg.LoadFile(filepath.Join(dir, DefaultConfigFile)); err != nil {
		t.Fatalf("LoadFile(init output): %v", err)
	}
	if cfg.Owner != "fordaj" || cfg.Repo != "example-repo" {
		t.Errorf("owner/repo = %v/%v, want fordaj/example-repo", cfg.Owner, cfg.Repo)
	}
}

func TestInitDiscoversCommittedConfig(t *testing.T) {
	// A committed .runner-spawner.json is the documented default: FindConfigFile
	// picks it up from the repo dir without any --config or env.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, DefaultConfigFile),
		[]byte(`{"owner":"fordaj","repo":"runner-spawner"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	found := FindConfigFile(dir)
	if found != filepath.Join(dir, DefaultConfigFile) {
		t.Fatalf("FindConfigFile(%q) = %q, want the committed .runner-spawner.json", dir, found)
	}
}
