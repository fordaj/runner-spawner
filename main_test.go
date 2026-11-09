package main

import (
	"bytes"
	"strings"
	"testing"
)

func clearSpawnerEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		EnvConfig, EnvRepoDir, EnvOwner, EnvRepo, EnvRunnerNames,
		EnvRunnersDir, EnvRunnerGroup, EnvRunnerVersion, EnvArchiveCacheDir,
		EnvLabels, EnvWindowsUsername, EnvWindowsPassword, EnvDryRun,
		EnvFormat, EnvQuiet,
	} {
		t.Setenv(k, "")
	}
}

func TestRun_PrintsRuntimeErrors(t *testing.T) {
	clearSpawnerEnv(t)
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	err := run([]string{"create", "--repo-dir", dir}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := stderr.String()
	if !strings.Contains(msg, "runner-spawner:") || !strings.Contains(msg, "owner/repo unknown") {
		t.Fatalf("stderr = %q, want runner-spawner error with owner/repo unknown", msg)
	}
	if exitCode(err) != 2 {
		t.Fatalf("exitCode = %d, want 2 for config error", exitCode(err))
	}
}

func TestRun_PrintsErrorsEvenWhenQuiet(t *testing.T) {
	clearSpawnerEnv(t)
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	err := run([]string{"list", "--repo-dir", dir, "--format", "bogus", "--quiet"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := stderr.String()
	if !strings.Contains(msg, "runner-spawner:") || !strings.Contains(msg, `invalid --format "bogus"`) {
		t.Fatalf("stderr = %q, want runner-spawner error even with --quiet", msg)
	}
}

func TestRun_UnknownSubcommandNotDuplicated(t *testing.T) {
	clearSpawnerEnv(t)
	var stdout, stderr bytes.Buffer
	err := run([]string{"bogus"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if exitCode(err) != 2 {
		t.Fatalf("exitCode = %d, want 2", exitCode(err))
	}
	msg := stderr.String()
	if strings.Count(msg, `unknown subcommand "bogus"`) != 1 {
		t.Fatalf("stderr = %q, want unknown subcommand mentioned exactly once", msg)
	}
	if strings.Contains(msg, "runner-spawner: unknown subcommand") {
		t.Fatalf("stderr = %q, central print must not duplicate the already-printed unknown error", msg)
	}
}

func TestRun_MissingSubcommandPrintsError(t *testing.T) {
	clearSpawnerEnv(t)
	var stdout, stderr bytes.Buffer
	err := run(nil, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(stderr.String(), "runner-spawner: missing subcommand") {
		t.Fatalf("stderr = %q, want central missing-subcommand error after usage", stderr.String())
	}
}

func TestRun_FlagErrorNotDuplicated(t *testing.T) {
	clearSpawnerEnv(t)
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	err := run([]string{"create", "--repo-dir", dir, "--nope"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if exitCode(err) != 2 {
		t.Fatalf("exitCode = %d, want 2", exitCode(err))
	}
	msg := stderr.String()
	if !strings.Contains(msg, "flag provided but not defined") {
		t.Fatalf("stderr = %q, want flag package error", msg)
	}
	if strings.Contains(msg, "runner-spawner: flag provided") {
		t.Fatalf("stderr = %q, central print must not duplicate the flag package error", msg)
	}
}
