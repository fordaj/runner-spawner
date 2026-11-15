package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveScopeMatrix(t *testing.T) {
	tests := []struct {
		name       string
		verb       string
		scope      string
		org        string
		github     bool
		cfgOwner   string
		cfgRepo    string
		wantLocal  string
		wantGitHub string
		wantErr    string
	}{
		{name: "default is directory", verb: "list", scope: "", wantLocal: "directory"},
		{name: "list pc", verb: "list", scope: "pc", wantLocal: "pc"},
		{name: "list repo", verb: "list", scope: "repo", wantLocal: "repo"},
		{name: "start repo", verb: "start", scope: "repo", wantLocal: "repo"},
		{name: "stop pc", verb: "stop", scope: "pc", wantLocal: "pc"},
		{name: "org scope is read-only for list", verb: "list", scope: "org", wantErr: "read-only GitHub inventory scope"},
		{name: "org scope is read-only for stop", verb: "stop", scope: "org", wantErr: "read-only GitHub inventory scope"},
		{name: "invalid scope", verb: "list", scope: "bogus", wantErr: "invalid --scope"},
		{name: "pc not supported for create", verb: "create", scope: "pc", wantErr: "not supported by create"},
		{name: "repo not supported for remove", verb: "remove", scope: "repo", wantErr: "not supported by remove"},
		{name: "directory for create", verb: "create", scope: "directory", wantLocal: "directory"},
		{name: "status pc", verb: "status", scope: "pc", wantLocal: "pc"},
		{name: "status org", verb: "status", scope: "org", wantGitHub: "org"},
		{name: "status owner", verb: "status", scope: "owner", wantGitHub: "owner"},
		{name: "status repo local", verb: "status", scope: "repo", wantLocal: "repo"},
		{name: "status repo github flag", verb: "status", scope: "repo", github: true, wantGitHub: "repo"},
		{name: "status repo owner/repo form implies github", verb: "status", scope: "", cfgOwner: "acme", cfgRepo: "acme/widgets", wantGitHub: "repo"},
		{name: "status repo explicit slashed --repo", verb: "status", scope: "repo", cfgOwner: "acme", cfgRepo: "acme/widgets", github: true, wantGitHub: "repo"},
		{name: "status github without scope", verb: "status", github: true, wantErr: "--github requires"},
		{name: "status pc with github flag", verb: "status", scope: "pc", github: true, wantErr: "--github is not supported with --scope pc"},
		{name: "list rejects org flag", verb: "list", org: "fordaj", wantErr: "--org only applies"},
		{name: "status repo rejects org flag", verb: "status", scope: "repo", org: "fordaj", wantErr: "--org only applies to --scope org"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Owner, cfg.Repo = tc.cfgOwner, tc.cfgRepo
			local, gh, err := resolveScope(tc.verb, tc.scope, tc.org, tc.github, cfg)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil (local=%q gh=%q)", tc.wantErr, local, gh)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %q, want substring %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if local != tc.wantLocal || gh != tc.wantGitHub {
				t.Fatalf("resolveScope = (%q, %q), want (%q, %q)", local, gh, tc.wantLocal, tc.wantGitHub)
			}
		})
	}
}

func TestResolveScopeSplitsSlashedRepo(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Owner, cfg.Repo = "", "acme/widgets"
	local, gh, err := resolveScope("status", "", "", false, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if gh != "repo" || local != "" {
		t.Fatalf("resolveScope = (%q, %q), want GitHub repo inventory", local, gh)
	}
	if cfg.Owner != "acme" || cfg.Repo != "widgets" {
		t.Fatalf("cfg owner/repo = %q/%q, want acme/widgets", cfg.Owner, cfg.Repo)
	}
}

// makeScopeInstallation creates a marker-backed installation in repoDir
// (under runnersDirRel) and registers it in the machine registry. The
// service is simulated as installed (plist) so service probes behave
// like a healthy, previously-created installation.
func makeScopeInstallation(t *testing.T, plat platform, repoDir, runnersDirRel, name, owner, repo string) string {
	t.Helper()
	cfg := DefaultConfig()
	cfg.RepoDir = repoDir
	cfg.RunnersDir = runnersDirRel
	cfg.Owner, cfg.Repo = owner, repo
	dir := filepath.Join(repoDir, runnersDirRel, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "svc.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	meta := buildInstallationMeta(cfg, plat, name, dir, "")
	if err := writeInstallMarker(dir, meta); err != nil {
		t.Fatal(err)
	}
	if err := registerInstallation(meta); err != nil {
		t.Fatal(err)
	}
	if !plat.IsWindows && plat.GOOS == "darwin" {
		home, err := userHomeDir()
		if err != nil {
			t.Fatal(err)
		}
		plist := macosServicePlist(home, cfg, name)
		if err := os.MkdirAll(filepath.Dir(plist), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(plist, []byte("<plist></plist>\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestStopScopePcReportsIndividualFailures(t *testing.T) {
	fx := newLifecycleFixture(t, "darwin")
	// The fixture installation lives in the fixture's repo, is running.
	if err := fx.create(); err != nil {
		t.Fatalf("create: %v", err)
	}

	// A second installation for the SAME repo lives in a different
	// working directory. Stopping it fails; the first must still stop.
	// NOTE: the fake shares one global service state, so this runner
	// must sort before the fixture's runner01 (discovery sorts by
	// name) to observe the pre-stop "running" state.
	cloneRoot := t.TempDir()
	cloneDir := filepath.Join(cloneRoot, "repo-clone")
	dirB := makeScopeInstallation(t, fx.plat, cloneDir, ".runners", "aaa-runnerB", "acme", "widgets")
	fx.fake.FailSvcStopDirs = map[string]error{dirB: fmt.Errorf("simulated stop failure")}

	var stdout, stderr bytes.Buffer
	err := runVerb("stop", []string{
		"--scope", "pc",
		"--repo-dir", fx.cfg.RepoDir,
		"--owner", "acme",
		"--repo", "widgets",
		"--quiet",
		"--format", "json",
	}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected aggregate error for the failing installation")
	}
	if !strings.Contains(err.Error(), "1 of 2") {
		t.Fatalf("error = %q, want aggregate count", err.Error())
	}

	var decoded struct {
		Scope   string           `json:"scope"`
		Results []mutationResult `json:"results"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON results: %v\n%s", err, stdout.String())
	}
	if decoded.Scope != "pc" {
		t.Errorf("scope = %q, want pc", decoded.Scope)
	}
	byName := map[string]mutationResult{}
	for _, r := range decoded.Results {
		byName[r.Name] = r
	}
	if len(byName) != 2 {
		t.Fatalf("results = %+v, want one per installation", decoded.Results)
	}
	if !byName[fx.name].OK {
		t.Errorf("fixture runner result = %+v, want ok", byName[fx.name])
	}
	if byName["aaa-runnerB"].OK {
		t.Errorf("aaa-runnerB result = %+v, want failure", byName["aaa-runnerB"])
	}
	if !strings.Contains(byName["aaa-runnerB"].Error, "simulated stop failure") {
		t.Errorf("aaa-runnerB error = %q, want the simulated failure", byName["aaa-runnerB"].Error)
	}
	// The healthy installation was actually stopped.
	if st := fx.fake.SvcState(); st != "stopped" {
		t.Errorf("svcState = %q, want stopped", st)
	}
}

func TestStartScopePcSucceeds(t *testing.T) {
	fx := newLifecycleFixture(t, "darwin")
	if err := fx.create(); err != nil {
		t.Fatalf("create: %v", err)
	}
	cloneDir := filepath.Join(t.TempDir(), "repo-clone")
	makeScopeInstallation(t, fx.plat, cloneDir, ".runners", "runnerB", "acme", "widgets")

	// Stop both so two starts are meaningful.
	var stdout, stderr bytes.Buffer
	if err := runVerb("stop", []string{
		"--scope", "pc", "--repo-dir", fx.cfg.RepoDir, "--owner", "acme", "--repo", "widgets", "--quiet",
	}, &stdout, &stderr); err != nil {
		t.Fatalf("stop pc: %v", err)
	}
	stdout.Reset()
	stderr.Reset()
	if err := runVerb("start", []string{
		"--scope", "pc", "--repo-dir", fx.cfg.RepoDir, "--owner", "acme", "--repo", "widgets", "--quiet", "--format", "json",
	}, &stdout, &stderr); err != nil {
		t.Fatalf("start pc: %v", err)
	}
	var decoded struct {
		Results []mutationResult `json:"results"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	for _, r := range decoded.Results {
		if !r.OK {
			t.Errorf("result = %+v, want ok", r)
		}
	}
}

func TestScopeOperationsWithEmptyRegistryAreNotAnError(t *testing.T) {
	clearSpawnerEnv(t)
	prevCfgDir := userConfigDir
	userConfigDir = func() (string, error) { return t.TempDir(), nil }
	t.Cleanup(func() { userConfigDir = prevCfgDir })
	repo := t.TempDir()
	var stdout, stderr bytes.Buffer
	err := runVerb("stop", []string{"--scope", "pc", "--repo-dir", repo, "--owner", "acme", "--repo", "widgets", "--quiet"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("stop --scope pc with nothing managed: %v", err)
	}
	if !strings.Contains(stderr.String(), "no runner-spawner-managed installations") {
		t.Errorf("stderr = %q, want the empty-scope note", stderr.String())
	}
}

func TestStatusScopeRepoReportsBothClones(t *testing.T) {
	fx := newLifecycleFixture(t, "darwin")
	if err := fx.create(); err != nil {
		t.Fatalf("create: %v", err)
	}
	cloneDir := filepath.Join(t.TempDir(), "repo-clone")
	makeScopeInstallation(t, fx.plat, cloneDir, ".runners", "runnerB", "acme", "widgets")

	var stdout, stderr bytes.Buffer
	if err := runVerb("status", []string{
		"--scope", "repo",
		"--repo-dir", fx.cfg.RepoDir,
		"--owner", "acme",
		"--repo", "widgets",
		"--quiet",
		"--format", "json",
	}, &stdout, &stderr); err != nil {
		t.Fatalf("status: %v", err)
	}
	var decoded struct {
		Scope   string         `json:"scope"`
		Runners []runnerStatus `json:"runners"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if decoded.Scope != "repo" {
		t.Errorf("scope = %q, want repo", decoded.Scope)
	}
	if len(decoded.Runners) != 2 {
		t.Fatalf("runners = %+v, want fixture runner + runnerB", decoded.Runners)
	}
	for _, r := range decoded.Runners {
		if r.Repo != "acme/widgets" {
			t.Errorf("runner %q repo = %q, want acme/widgets", r.Name, r.Repo)
		}
	}
}

func TestListScopePcPrintsRepoColumns(t *testing.T) {
	fx := newLifecycleFixture(t, "darwin")
	if err := fx.create(); err != nil {
		t.Fatalf("create: %v", err)
	}
	var stdout, stderr bytes.Buffer
	if err := runVerb("list", []string{
		"--scope", "pc",
		"--repo-dir", fx.cfg.RepoDir,
		"--owner", "acme",
		"--repo", "widgets",
		"--quiet",
	}, &stdout, &stderr); err != nil {
		t.Fatalf("list: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "repo=acme/widgets") {
		t.Errorf("output %q missing repo=acme/widgets", out)
	}
	if !strings.Contains(out, fx.name) {
		t.Errorf("output %q missing runner name", out)
	}
}

func TestBatchDirectoryScopeContinues(t *testing.T) {
	// Even in the classic directory scope, one failing name must not
	// abort the batch.
	fx := newLifecycleFixture(t, "darwin")
	if err := fx.create(); err != nil {
		t.Fatalf("create: %v", err)
	}
	cloneDir := filepath.Join(t.TempDir(), "repo-clone")
	makeScopeInstallation(t, fx.plat, cloneDir, ".runners", "runnerB", "acme", "widgets")

	// Both names in the fixture's repo: runner01 (present) and
	// nonexistent (absent). The nonexistent one fails; the present one
	// still stops.
	var stdout, stderr bytes.Buffer
	err := runVerb("stop", []string{
		"--repo-dir", fx.cfg.RepoDir,
		"--owner", "acme",
		"--repo", "widgets",
		"--name", fx.name,
		"--name", "missing-runner",
		"--quiet",
		"--format", "json",
	}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected aggregate error")
	}
	if !strings.Contains(err.Error(), "1 of 2") {
		t.Fatalf("error = %q, want 1 of 2", err.Error())
	}
	var decoded struct {
		Results []mutationResult `json:"results"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Results) != 2 {
		t.Fatalf("results = %+v, want both names", decoded.Results)
	}
	if !decoded.Results[0].OK {
		t.Errorf("fixture runner should have stopped: %+v", decoded.Results[0])
	}
	if decoded.Results[1].OK {
		t.Errorf("missing runner should have failed: %+v", decoded.Results[1])
	}
}

func TestCreateWrongScopeFails(t *testing.T) {
	fx := newLifecycleFixture(t, "darwin")
	var stdout, stderr bytes.Buffer
	err := runVerb("create", []string{
		"--scope", "pc",
		"--repo-dir", fx.cfg.RepoDir,
		"--owner", "acme",
		"--repo", "widgets",
		"--quiet",
	}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for create --scope pc")
	}
	if !strings.Contains(err.Error(), "not supported by create") {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestStopAlreadyStoppedIsIdempotentScoped(t *testing.T) {
	fx := newLifecycleFixture(t, "darwin")
	if err := fx.create(); err != nil {
		t.Fatalf("create: %v", err)
	}
	args := []string{
		"--scope", "pc", "--repo-dir", fx.cfg.RepoDir, "--owner", "acme", "--repo", "widgets", "--quiet",
	}
	var stdout, stderr bytes.Buffer
	if err := runVerb("stop", append(args, "--format", "json"), &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	// Stopping an already-stopped runner is safe.
	stdout.Reset()
	stderr.Reset()
	if err := runVerb("stop", append(args, "--format", "json"), &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Results []mutationResult `json:"results"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	for _, r := range decoded.Results {
		if !r.OK {
			t.Errorf("result = %+v, want ok", r)
		}
	}
}
