package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// installTestEnv wires a temp machine-wide config dir (registry), three
// repo clones (two of them clones of the same repository), and redirects
// nothing else. Discovery tests work directly on the registry + markers.
type installTestEnv struct {
	cfgDir string
	repoA  string // clone 1 of acme/repoa
	repoA2 string // clone 2 of acme/repoa (different working directory)
	repoB  string // unrelated repo acme/repob
	plat   platform
}

func newInstallTestEnv(t *testing.T) *installTestEnv {
	t.Helper()
	clearSpawnerEnv(t)
	prev := userConfigDir
	cfgDir := t.TempDir()
	userConfigDir = func() (string, error) { return cfgDir, nil }
	t.Cleanup(func() { userConfigDir = prev })
	root := t.TempDir()
	e := &installTestEnv{
		cfgDir: cfgDir,
		repoA:  filepath.Join(root, "repoa"),
		repoA2: filepath.Join(root, "repoa-clone2"),
		repoB:  filepath.Join(root, "repob"),
		plat:   platformFor("darwin", "arm64", "testhost"),
	}
	for _, d := range []string{e.repoA, e.repoA2, e.repoB} {
		if err := os.MkdirAll(filepath.Join(d, ".runners"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return e
}

// discoveryCfg returns a Config bound to repoA.
func (e *installTestEnv) discoveryCfg() *Config {
	cfg := DefaultConfig()
	cfg.RepoDir = e.repoA
	cfg.RunnersDir = ".runners"
	cfg.Owner, cfg.Repo = "acme", "repoa"
	return cfg
}

// makeInstallation creates a runner-spawner-managed installation: a
// marker inside dir, plus (when register is true) a registry entry.
func (e *installTestEnv) makeInstallation(t *testing.T, repoDir, name, owner, repo string, marker, register bool) InstallationMeta {
	t.Helper()
	cfg := DefaultConfig()
	cfg.RepoDir = repoDir
	cfg.Owner, cfg.Repo = owner, repo
	dir := filepath.Join(repoDir, ".runners", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta := buildInstallationMeta(cfg, e.plat, name, dir, "")
	if marker {
		if err := writeInstallMarker(dir, meta); err != nil {
			t.Fatal(err)
		}
	}
	if register {
		if err := registerInstallation(meta); err != nil {
			t.Fatal(err)
		}
	}
	return meta
}

// makeRootRunner creates an on-disk runner dir in a repo's .runners root
// without a marker (e.g. a pre-registry installation).
func (e *installTestEnv) makeRootRunner(t *testing.T, repoDir, name string) string {
	t.Helper()
	dir := filepath.Join(repoDir, ".runners", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestInstallMarkerRoundTrip(t *testing.T) {
	e := newInstallTestEnv(t)
	meta := e.makeInstallation(t, e.repoA, "runner01", "acme", "repoa", true, false)
	dir := meta.InstallationDir
	got, err := readInstallMarker(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "runner01" || got.Tool != installTool || got.RepoFullName != "acme/repoa" {
		t.Errorf("unexpected marker: %+v", got)
	}
	if got.InstallationDir != dir {
		t.Errorf("InstallationDir = %q, want %q", got.InstallationDir, dir)
	}
	if got.Platform != "darwin" || got.Architecture != "arm64" {
		t.Errorf("platform = %s/%s, want darwin/arm64", got.Platform, got.Architecture)
	}
	if got.Service.Type == "" || got.Service.Name == "" {
		t.Errorf("service metadata missing: %+v", got.Service)
	}
	if _, err := time.Parse(time.RFC3339, got.CreatedAt); err != nil {
		t.Errorf("CreatedAt %q is not RFC3339: %v", got.CreatedAt, err)
	}
	// Re-creating must preserve CreatedAt but bump nothing else.
	preserved := got.CreatedAt
	meta2 := buildInstallationMeta(mustLoadCfg(t, e.repoA, "acme", "repoa"), e.plat, "runner01", dir, preserved)
	if meta2.CreatedAt != preserved {
		t.Errorf("CreatedAt not preserved across re-create: %q vs %q", meta2.CreatedAt, preserved)
	}
}

func mustLoadCfg(t *testing.T, repoDir, owner, repo string) *Config {
	t.Helper()
	cfg := DefaultConfig()
	cfg.RepoDir = repoDir
	cfg.Owner = owner
	cfg.Repo = repo
	return cfg
}

func TestReadInstallMarkerRejectsForeignTool(t *testing.T) {
	e := newInstallTestEnv(t)
	meta := e.makeInstallation(t, e.repoA, "runner01", "acme", "repoa", true, false)
	data, err := json.Marshal(map[string]any{"tool": "not-spawner", "name": "runner01"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(meta.InstallationDir, installMarkerFile), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readInstallMarker(meta.InstallationDir); err == nil {
		t.Fatal("expected error for foreign tool marker")
	}
}

func TestReadInstallMarkerRejectsNameMismatch(t *testing.T) {
	e := newInstallTestEnv(t)
	meta := e.makeInstallation(t, e.repoA, "runner01", "acme", "repoa", true, false)
	data, err := os.ReadFile(filepath.Join(meta.InstallationDir, installMarkerFile))
	if err != nil {
		t.Fatal(err)
	}
	var m InstallationMeta
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	m.Name = "different"
	out, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(meta.InstallationDir, installMarkerFile), out, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readInstallMarker(meta.InstallationDir); err == nil {
		t.Fatal("expected error for name mismatch")
	}
}

func TestRegistryMissingIsEmpty(t *testing.T) {
	_ = newInstallTestEnv(t)
	reg, err := loadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if len(reg.Installations) != 0 {
		t.Fatalf("expected empty registry, got %d installations", len(reg.Installations))
	}
}

func TestRegistryCorruptFailsLoudly(t *testing.T) {
	e := newInstallTestEnv(t)
	path, err := registryPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadRegistry(); err == nil {
		t.Fatal("expected corrupt registry to be an error")
	}
	// Discovery must not silently manage a subset: it errors instead.
	if _, _, err := discoverInstallations(e.discoveryCfg(), e.plat, "pc"); err == nil {
		t.Fatal("expected discoverInstallations to surface the corrupt registry")
	}
}

func TestRegistryUpsertUnregister(t *testing.T) {
	e := newInstallTestEnv(t)
	m1 := e.makeInstallation(t, e.repoA, "runner-a", "acme", "repoa", true, true)
	m2 := e.makeInstallation(t, e.repoB, "runner-b", "acme", "repob", true, true)

	reg, err := loadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if len(reg.Installations) != 2 {
		t.Fatalf("registry has %d installations, want 2", len(reg.Installations))
	}

	// Re-registering the same dir keeps one entry and preserves CreatedAt.
	updated := m1
	updated.Name = "runner-a"
	updated.CreatedAt = m1.CreatedAt
	if err := registerInstallation(updated); err != nil {
		t.Fatal(err)
	}
	reg, err = loadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if len(reg.Installations) != 2 {
		t.Fatalf("after upsert registry has %d installations, want 2", len(reg.Installations))
	}

	// Unregister removes exactly one; unknown dirs are a no-op.
	if err := unregisterInstallation(m1.InstallationDir); err != nil {
		t.Fatal(err)
	}
	reg, err = loadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if len(reg.Installations) != 1 || reg.Installations[0].Name != "runner-b" {
		t.Fatalf("after unregister registry = %+v, want only runner-b", reg.Installations)
	}
	if err := unregisterInstallation(filepath.Join(e.repoA, "does-not-exist")); err != nil {
		t.Fatal(err)
	}
	_ = m2
}

func TestDiscoverDirectoryScopeIgnoresRegistry(t *testing.T) {
	e := newInstallTestEnv(t)
	e.makeRootRunner(t, e.repoA, "root-runner")
	e.makeInstallation(t, e.repoB, "repob-runner", "acme", "repob", true, true)
	found, warnings, err := discoverInstallations(e.discoveryCfg(), e.plat, "directory")
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].Name != "root-runner" {
		t.Fatalf("directory scope = %+v, want only root-runner", found)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
}

func TestDiscoverRepoScopeFindsOtherWorkingDirs(t *testing.T) {
	e := newInstallTestEnv(t)
	e.makeRootRunner(t, e.repoA, "root-runner")
	e.makeInstallation(t, e.repoA2, "clone-runner", "acme", "repoa", true, true) // same repo, second clone
	e.makeInstallation(t, e.repoB, "other-repo-runner", "acme", "repob", true, true)

	found, warnings, err := discoverInstallations(e.discoveryCfg(), e.plat, "repo")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for _, d := range found {
		names[d.Name] = d.From
	}
	if len(names) != 2 {
		t.Fatalf("repo scope found %v, want root-runner and clone-runner", names)
	}
	if names["root-runner"] != "root" || names["clone-runner"] != "registry" {
		t.Fatalf("discovery sources = %v, want root + registry", names)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
}

func TestDiscoverRepoScopeMatchesByRepoDirWithoutGitRemote(t *testing.T) {
	e := newInstallTestEnv(t)
	cfg := e.discoveryCfg()
	cfg.Owner, cfg.Repo = "", "" // no git remote, no config: match by dir only
	e.makeInstallation(t, e.repoA2, "clone-runner", "acme", "repoa", true, true)
	found, _, err := discoverInstallations(cfg, e.plat, "repo")
	if err != nil {
		t.Fatal(err)
	}
	foundNames := []string{}
	for _, d := range found {
		foundNames = append(foundNames, d.Name)
	}
	// repoA2 is a *different directory*, so it can only match by repoFullName;
	// empty owner/repo on the invocation means it must NOT match.
	if len(foundNames) != 0 {
		t.Fatalf("repo scope matched %v with no owner/repo anywhere to compare", foundNames)
	}
	cfg.Owner, cfg.Repo = "ACME", "REPOA" // case-insensitive match
	found, _, err = discoverInstallations(cfg, e.plat, "repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].Name != "clone-runner" {
		t.Fatalf("case-insensitive repo match found %+v, want clone-runner", found)
	}
}

func TestDiscoverPcScopeEverythingButOnlyManaged(t *testing.T) {
	e := newInstallTestEnv(t)
	// A valid installation in the current root, also registered (dedup).
	rootMeta := e.makeInstallation(t, e.repoA, "managed-root", "acme", "repoa", true, true)
	// Valid one in the other repo.
	e.makeInstallation(t, e.repoB, "managed-b", "acme", "repob", true, true)
	// Stale: registered, but dir is gone.
	stale := e.makeInstallation(t, e.repoA2, "stale-runner", "acme", "repoa", true, true)
	if err := os.RemoveAll(stale.InstallationDir); err != nil {
		t.Fatal(err)
	}
	// Unverifiable: registered, but marker is missing.
	e.makeInstallation(t, e.repoA2, "unmanaged-runner", "acme", "repoa", false, true)
	// Root runner with no marker at all (pre-registry install) — still
	// in scope because it lives under this tool's configured root.
	e.makeRootRunner(t, e.repoA, "untracked-root")

	found, warnings, err := discoverInstallations(e.discoveryCfg(), e.plat, "pc")
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, d := range found {
		names = append(names, d.Name)
	}
	want := map[string]bool{"managed-root": true, "managed-b": true, "untracked-root": true}
	if len(names) != len(want) {
		t.Fatalf("pc scope found %v, want %v (warnings: %v)", names, want, warnings)
	}
	for _, n := range names {
		if !want[n] {
			t.Fatalf("pc scope unexpectedly found %q (managed-root dir %s)", n, rootMeta.InstallationDir)
		}
	}
	// The stale and unverifiable entries are skipped, never managed.
	if len(warnings) != 2 {
		t.Fatalf("warnings = %v, want 2 (stale + unverifiable)", warnings)
	}
	for _, w := range warnings {
		if !strings.Contains(w, "stale-runner") && !strings.Contains(w, "unmanaged-runner") {
			t.Fatalf("unexpected warning: %q", w)
		}
	}
}

func TestDiscoverPcScopeRootRunnerWinsOverRegistry(t *testing.T) {
	e := newInstallTestEnv(t)
	// Same installation registered AND visible in the current root:
	// reported once, sourced from the root.
	e.makeInstallation(t, e.repoA, "managed-root", "acme", "repoa", true, true)
	found, _, err := discoverInstallations(e.discoveryCfg(), e.plat, "pc")
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 {
		t.Fatalf("found %d installations, want 1 (deduped)", len(found))
	}
	if found[0].From != "root" {
		t.Fatalf("From = %q, want root", found[0].From)
	}
}

func TestFilterByNames(t *testing.T) {
	found := []discoveredRunner{
		{InstallationMeta: InstallationMeta{Name: "a"}},
		{InstallationMeta: InstallationMeta{Name: "b"}},
		{InstallationMeta: InstallationMeta{Name: "c"}},
	}
	got := filterByNames(found, []string{"b"})
	if len(got) != 1 || got[0].Name != "b" {
		t.Fatalf("filterByNames = %+v, want only b", got)
	}
	if got := filterByNames(found, nil); len(got) != 3 {
		t.Fatalf("filterByNames(nil) = %d, want all", len(got))
	}
}

func TestScopedConfigBindsToInstallation(t *testing.T) {
	e := newInstallTestEnv(t)
	meta := e.makeInstallation(t, e.repoA2, "clone-runner", "acme", "repoa", true, true)
	sc := scopedConfig(e.discoveryCfg(), discoveredRunner{InstallationMeta: meta, From: "registry"})
	if sc.RepoDir != e.repoA2 {
		t.Errorf("RepoDir = %q, want %q", sc.RepoDir, e.repoA2)
	}
	if filepath.Dir(meta.InstallationDir) != sc.RunnersDir {
		t.Errorf("RunnersDir = %q, want %q", sc.RunnersDir, filepath.Dir(meta.InstallationDir))
	}
	if sc.Owner != "acme" || sc.Repo != "repoa" {
		t.Errorf("owner/repo = %q/%q, want acme/repoa", sc.Owner, sc.Repo)
	}
	// The scoped config must point runnerDirFor at the installation dir.
	dir, err := runnerDirFor(sc, "clone-runner")
	if err != nil {
		t.Fatal(err)
	}
	if dir != meta.InstallationDir {
		t.Errorf("runnerDirFor = %q, want %q", dir, meta.InstallationDir)
	}
}

func TestDiscoveryErrorsOnRegistryWithMismatchedMarker(t *testing.T) {
	e := newInstallTestEnv(t)
	// Registry entry whose marker disagrees with the registry: skipped
	// with a warning, never managed.
	meta := e.makeInstallation(t, e.repoB, "managed-b", "acme", "repob", true, true)
	raw, err := os.ReadFile(filepath.Join(meta.InstallationDir, installMarkerFile))
	if err != nil {
		t.Fatal(err)
	}
	var marker InstallationMeta
	if err := json.Unmarshal(raw, &marker); err != nil {
		t.Fatal(err)
	}
	marker.RepoFullName = "acme/other"
	marker.Repo = "other"
	raw, _ = json.Marshal(marker)
	if err := os.WriteFile(filepath.Join(meta.InstallationDir, installMarkerFile), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	found, warnings, err := discoverInstallations(e.discoveryCfg(), e.plat, "pc")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range found {
		if d.Name == "managed-b" {
			t.Fatalf("mismatched installation was discovered: %+v", d)
		}
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "managed-b") {
		t.Fatalf("warnings = %v, want one mentioning the mismatched entry", warnings)
	}
}
