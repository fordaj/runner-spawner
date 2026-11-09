package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.RepoDir != "." {
		t.Fatalf("RepoDir = %q, want .", cfg.RepoDir)
	}
	if cfg.RunnersDir != ".runners" {
		t.Fatalf("RunnersDir = %q, want .runners", cfg.RunnersDir)
	}
	if cfg.RunnerGroup != "Default" {
		t.Fatalf("RunnerGroup = %q, want Default", cfg.RunnerGroup)
	}
	if cfg.RunnerVersion != "2.338.2" {
		t.Fatalf("RunnerVersion = %q, want 2.338.2", cfg.RunnerVersion)
	}
}

func TestLoadFile_OverlayAndUnknown(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".runner-spawner.json")
	if err := os.WriteFile(path, []byte(`{
  "owner": "fordaj",
  "repo": "AirFrame",
  "runnerNames": ["runner01", "runner02"],
  "runnerGroup": "Default",
  "runnerVersion": "2.338.2"
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	if err := cfg.LoadFile(path); err != nil {
		t.Fatal(err)
	}
	if cfg.Owner != "fordaj" || cfg.Repo != "AirFrame" {
		t.Fatalf("owner/repo = %q/%q, want fordaj/AirFrame", cfg.Owner, cfg.Repo)
	}
	if len(cfg.RunnerNames) != 2 || cfg.RunnerNames[0] != "runner01" {
		t.Fatalf("runnerNames = %+v, want [runner01 runner02]", cfg.RunnerNames)
	}
	if cfg.RunnersDir != ".runners" {
		t.Fatalf("RunnersDir = %q, want default .runners", cfg.RunnersDir)
	}

	// Unknown keys must fail fast (typo in committed file).
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte(`{"owner":"x","runerNames":["a"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg2 := DefaultConfig()
	if err := cfg2.LoadFile(bad); err == nil {
		t.Fatal("expected error for unknown key runerNames, got nil")
	}
}

func TestApplyEnvAndPrecedence(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Owner = "from-config"
	cfg.Repo = "cfg-repo"
	cfg.RunnerNames = []string{"cfg-runner"}
	getenv := func(k string) string {
		switch k {
		case EnvOwner:
			return "from-env"
		case EnvRunnerNames:
			return " runner01 , runner02 ,"
		case EnvRunnerVersion:
			return "2.330.0"
		case EnvDryRun:
			return "true"
		}
		return ""
	}
	if err := cfg.ApplyEnv(getenv); err != nil {
		t.Fatal(err)
	}
	if cfg.Owner != "from-env" {
		t.Fatalf("Owner = %q, want from-env (env must override file)", cfg.Owner)
	}
	if len(cfg.RunnerNames) != 2 || cfg.RunnerNames[1] != "runner02" {
		t.Fatalf("RunnerNames = %+v, want [runner01 runner02] (env must override file)", cfg.RunnerNames)
	}
	if cfg.RunnerVersion != "2.330.0" {
		t.Fatalf("RunnerVersion = %q, want 2.330.0", cfg.RunnerVersion)
	}
	if !cfg.DryRun {
		t.Fatal("DryRun = false, want true from env")
	}
	// Invalid booleans are a hard error, not silently ignored.
	cfgBad := DefaultConfig()
	if err := cfgBad.ApplyEnv(func(k string) string {
		if k == EnvQuiet {
			return "yes"
		}
		return ""
	}); err == nil {
		t.Fatal("expected error for non-boolean RUNNER_SPAWNER_QUIET, got nil")
	}
}

func TestResolveNames_FlagsWin(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RunnerNames = []string{"from-config"}
	if got := cfg.ResolveNames([]string{"from-flags"}); len(got) != 1 || got[0] != "from-flags" {
		t.Fatalf("ResolveNames(flags) = %+v, want [from-flags]", got)
	}
	if got := cfg.ResolveNames(nil); len(got) != 1 || got[0] != "from-config" {
		t.Fatalf("ResolveNames(nil) = %+v, want [from-config]", got)
	}
}

func TestParseList(t *testing.T) {
	if got := ParseList(" runner01 , runner02 ,"); len(got) != 2 || got[0] != "runner01" || got[1] != "runner02" {
		t.Fatalf("ParseList = %+v, want [runner01 runner02]", got)
	}
	if got := ParseList(""); len(got) != 0 {
		t.Fatalf("ParseList(\"\") = %+v, want []", got)
	}
}

func TestValidate(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(true, nil); err == nil {
		t.Fatal("Validate(true) without owner/repo: expected error, got nil")
	}
	cfg.Owner, cfg.Repo = "fordaj", "AirFrame"
	if err := cfg.Validate(true, nil); err != nil {
		t.Fatalf("Validate(true) with owner/repo: %v", err)
	}
	if err := cfg.Validate(false, nil); err != nil {
		t.Fatalf("Validate(false) without repo should still pass: %v", err)
	}
}

func TestFindConfigFile(t *testing.T) {
	dir := t.TempDir()
	if got := FindConfigFile(dir); got != "" {
		t.Fatalf("FindConfigFile(empty) = %q, want \"\"", got)
	}
	path := filepath.Join(dir, ".runner-spawner.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := FindConfigFile(dir); got != path {
		t.Fatalf("FindConfigFile = %q, want %q", got, path)
	}
}
