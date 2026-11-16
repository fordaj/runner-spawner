package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// This file implements scope-aware discovery of runner-spawner-managed
// installations on the local machine.
//
// Every installation created by this tool carries a marker file,
// .runner-spawner-install.json, inside the runner dir. The marker holds
// the identity of the installation (runner name, owning repository,
// installation dir, platform, service registration). A machine-wide
// registry, <user-config-dir>/runner-spawner/installations.json, lists
// the installation dirs so pc-wide operations can find installations
// outside the current repo's .runners directory.
//
// PC-wide operations only target installations that are (a) listed in
// the registry and (b) still valid — the dir exists and its marker
// parses and was written by runner-spawner. Anything else is reported
// as skipped, never managed. Installations inside the current runners
// root are always in scope (list/status semantics of this tool) and do
// not need the marker; they are managed because they live under this
// tool's configured root.
//
// The registry is a convenience index, not a source of truth: the marker
// inside the dir is authoritative. Losing the registry just limits pc
// discovery to the current root; it never causes this tool to manage a
// directory it cannot identify.

const (
	installMarkerFile  = ".runner-spawner-install.json"
	installTool        = "runner-spawner"
	installMetaVersion = 1

	registryDirName  = "runner-spawner"
	registryFileName = "installations.json"
	registryLockName = "installations.lock"
	registryVersion  = 1
)

// InstallationMeta describes one runner-spawner-managed installation.
// It is written both inside the runner dir (the marker, authoritative)
// and in the machine registry (an index of installation dirs).
type InstallationMeta struct {
	Version           int         `json:"version"`
	Tool              string      `json:"tool"` // always "runner-spawner"
	Name              string      `json:"name"`
	DisplayName       string      `json:"displayName"`
	Owner             string      `json:"owner"`
	Repo              string      `json:"repo"`
	RepoFullName      string      `json:"repoFullName"` // owner/repo
	RepoDir           string      `json:"repoDir"`      // absolute path of the owning clone
	InstallationDir   string      `json:"installationDir"`
	Platform          string      `json:"platform"`     // GOOS, e.g. darwin|windows|linux
	Architecture      string      `json:"architecture"` // GOARCH, e.g. arm64|amd64
	RegistrationScope string      `json:"registrationScope"`
	Service           ServiceMeta `json:"service"`
	CreatedAt         string      `json:"createdAt"` // RFC3339
	UpdatedAt         string      `json:"updatedAt"` // RFC3339
}

// ServiceMeta records how the installation is registered as a service,
// so operators can identify what to check when something looks wrong.
type ServiceMeta struct {
	Type string `json:"type"` // svc-sh | launchd | windows-service | unsupported
	Name string `json:"name"` // sc.exe service name / LaunchAgent label / ""
}

// registryFile is the on-disk shape of the machine-wide index.
type registryFile struct {
	Version       int                `json:"version"`
	Installations []InstallationMeta `json:"installations"`
}

// userConfigDir is the parent of the runner-spawner config dir. It is a
// seam so tests can redirect the registry into a temp dir.
var userConfigDir = os.UserConfigDir

func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339) }

// serviceMetaFor describes how the platform registers the service.
func serviceMetaFor(cfg *Config, p platform, name string) ServiceMeta {
	switch {
	case p.IsWindows:
		return ServiceMeta{Type: "windows-service", Name: serviceName(cfg, name)}
	case p.GOOS == "darwin":
		// svc.sh is backed by a per-user LaunchAgent plist.
		return ServiceMeta{Type: "svc-sh", Name: serviceName(cfg, name)}
	case p.GOOS == "linux":
		return ServiceMeta{Type: "svc-sh", Name: ""}
	default:
		return ServiceMeta{Type: "unsupported", Name: ""}
	}
}

// buildInstallationMeta synthesizes the metadata for an installation.
// createdAt is preserved across re-creates when a marker already exists.
func buildInstallationMeta(cfg *Config, p platform, name, dir, createdAt string) InstallationMeta {
	dirClean := filepath.Clean(dir)
	repoDir := cfg.RepoDir
	if abs, err := filepath.Abs(repoDir); err == nil {
		repoDir = abs
	}
	repoDir = filepath.Clean(repoDir)
	full := repoFullName(cfg)
	if createdAt == "" {
		createdAt = nowRFC3339()
	}
	return InstallationMeta{
		Version:           installMetaVersion,
		Tool:              installTool,
		Name:              name,
		DisplayName:       displayName(name),
		Owner:             cfg.Owner,
		Repo:              cfg.Repo,
		RepoFullName:      full,
		RepoDir:           repoDir,
		InstallationDir:   dirClean,
		Platform:          p.GOOS,
		Architecture:      p.GOARCH,
		RegistrationScope: "repo",
		Service:           serviceMetaFor(cfg, p, name),
		CreatedAt:         createdAt,
		UpdatedAt:         nowRFC3339(),
	}
}

// writeInstallMarker atomically writes dir/.runner-spawner-install.json.
func writeInstallMarker(dir string, meta InstallationMeta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(filepath.Join(dir, installMarkerFile), data, 0o644)
}

// readInstallMarker parses and validates the marker in dir.
func readInstallMarker(dir string) (InstallationMeta, error) {
	var meta InstallationMeta
	data, err := os.ReadFile(filepath.Join(dir, installMarkerFile))
	if err != nil {
		return meta, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&meta); err != nil {
		return InstallationMeta{}, fmt.Errorf("%s: %w", installMarkerFile, err)
	}
	if meta.Tool != installTool {
		return InstallationMeta{}, fmt.Errorf("%s: unknown tool %q (this installation is not managed by runner-spawner)", installMarkerFile, meta.Tool)
	}
	if meta.Name == "" {
		return InstallationMeta{}, fmt.Errorf("%s: missing runner name", installMarkerFile)
	}
	if !pathEqual(meta.InstallationDir, dir) {
		return InstallationMeta{}, fmt.Errorf("%s: installation dir %q does not match %q", installMarkerFile, meta.InstallationDir, dir)
	}
	if meta.Name != filepath.Base(filepath.Clean(dir)) {
		return InstallationMeta{}, fmt.Errorf("%s: runner name %q does not match directory name %q", installMarkerFile, meta.Name, filepath.Base(filepath.Clean(dir)))
	}
	return meta, nil
}

// atomicWriteFile writes data to path via a temp file + rename so a
// crash never leaves a half-written marker or registry behind.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-"+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

// registryPath returns the absolute path of the machine-wide registry.
func registryPath() (string, error) {
	cfgDir, err := userConfigDir()
	if err != nil {
		return "", fmt.Errorf("locating user config dir: %w", err)
	}
	return filepath.Join(cfgDir, registryDirName, registryFileName), nil
}

// loadRegistry reads the registry. A missing registry is an empty one;
// a corrupt registry is an error (callers must not silently manage a
// subset of installations).
func loadRegistry() (registryFile, error) {
	var reg registryFile
	path, err := registryPath()
	if err != nil {
		return reg, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return registryFile{Version: registryVersion}, nil
	}
	if err != nil {
		return reg, err
	}
	if err := json.Unmarshal(data, &reg); err != nil {
		return registryFile{}, fmt.Errorf("%s: %w", path, err)
	}
	if reg.Version == 0 {
		reg.Version = registryVersion
	}
	return reg, nil
}

// withRegistryLock serializes registry read-modify-write cycles across
// concurrent runner-spawner processes on this machine.
func withRegistryLock(fn func() error) error {
	path, err := registryPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	lockPath := filepath.Join(filepath.Dir(path), registryLockName)
	if err := acquireLockFile(lockPath, "installations-registry"); err != nil {
		return err
	}
	defer func() { _ = os.Remove(lockPath) }()
	return fn()
}

// saveRegistry atomically writes the registry (caller holds the lock).
func saveRegistry(reg registryFile) error {
	path, err := registryPath()
	if err != nil {
		return err
	}
	reg.Version = registryVersion
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(path, data, 0o644)
}

// registerInstallation upserts an installation into the machine registry.
func registerInstallation(meta InstallationMeta) error {
	meta.InstallationDir = filepath.Clean(meta.InstallationDir)
	meta.RepoDir = filepath.Clean(meta.RepoDir)
	return withRegistryLock(func() error {
		reg, err := loadRegistry()
		if err != nil {
			return err
		}
		found := false
		for i, m := range reg.Installations {
			if pathEqual(m.InstallationDir, meta.InstallationDir) {
				// Preserve the original creation timestamp.
				if meta.CreatedAt == "" {
					meta.CreatedAt = m.CreatedAt
				}
				reg.Installations[i] = meta
				found = true
				break
			}
		}
		if !found {
			reg.Installations = append(reg.Installations, meta)
		}
		sort.Slice(reg.Installations, func(i, j int) bool {
			a, b := reg.Installations[i], reg.Installations[j]
			if a.InstallationDir == b.InstallationDir {
				return a.Name < b.Name
			}
			return a.InstallationDir < b.InstallationDir
		})
		return saveRegistry(reg)
	})
}

// unregisterInstallation removes an installation dir from the registry.
// A missing entry is not an error.
func unregisterInstallation(dir string) error {
	clean := filepath.Clean(dir)
	return withRegistryLock(func() error {
		reg, err := loadRegistry()
		if err != nil {
			return err
		}
		kept := reg.Installations[:0]
		for _, m := range reg.Installations {
			if !pathEqual(m.InstallationDir, clean) {
				kept = append(kept, m)
			}
		}
		reg.Installations = kept
		return saveRegistry(reg)
	})
}

// discoveredRunner is one installation found by discovery. From records
// how it was found: "root" (the configured runners dir) or "registry"
// (the machine-wide index, validated against its marker).
type discoveredRunner struct {
	InstallationMeta
	From string `json:"from"`
}

// repoFullNamesEqual compares two owner/repo strings case-insensitively.
func repoFullNamesEqual(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// pathEqual compares two filesystem paths. Case-insensitive on Windows,
// exact otherwise (Linux filesystems may be case-sensitive).
func pathEqual(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// repoMatches reports whether an installation belongs to the repo this
// command was invoked for. Match by canonical repo dir first (works even
// without a git remote), then by owner/repo full name.
func repoMatches(cfg *Config, m InstallationMeta) bool {
	if cfgAbs, err := cfg.AbsRepoDir(); err == nil {
		if pathEqual(m.RepoDir, cfgAbs) {
			return true
		}
	}
	want := repoFullName(cfg)
	return want != "" && m.RepoFullName != "" && repoFullNamesEqual(want, m.RepoFullName)
}

// synthesizedMeta builds metadata for an installation in the current
// runners root that may not (yet) carry a marker — e.g. runners created
// before the marker existed. Identity falls back to the current config.
func synthesizedMeta(cfg *Config, p platform, dir string) InstallationMeta {
	name := filepath.Base(filepath.Clean(dir))
	m := buildInstallationMeta(cfg, p, name, dir, "")
	m.CreatedAt = "" // unknown
	m.UpdatedAt = ""
	return m
}

// validRegistryEntry checks a registry entry against its on-dir marker.
// The marker is authoritative: the entry is used only when both agree.
// Returns the marker metadata, a human-readable skip reason, and ok.
func validRegistryEntry(m InstallationMeta) (InstallationMeta, string, bool) {
	dir := filepath.Clean(m.InstallationDir)
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		return InstallationMeta{}, fmt.Sprintf("registry entry %q skipped: directory is missing", dir), false
	}
	marker, err := readInstallMarker(dir)
	if err != nil {
		return InstallationMeta{}, fmt.Sprintf("registry entry %q skipped: %v", dir, err), false
	}
	if !strings.EqualFold(marker.Name, m.Name) || !pathEqual(marker.RepoDir, m.RepoDir) || !repoFullNamesEqual(marker.RepoFullName, m.RepoFullName) {
		return InstallationMeta{}, fmt.Sprintf("registry entry %q skipped: marker does not match registry (name %q vs %q, repo %q vs %q)", dir, marker.Name, m.Name, marker.RepoFullName, m.RepoFullName), false
	}
	return marker, "", true
}

// rootInstallations enumerates installations under the configured runners
// root — always in scope. Entries without a readable marker are adopted via
// synthesizedMeta: they are managed because they live under this tool's
// configured root, not because they carry proof of identity.
func rootInstallations(cfg *Config, p platform) []InstallationMeta {
	meta := []InstallationMeta{}
	root, err := cfg.RunnersRoot()
	if err != nil {
		return meta
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return meta
	}
	for _, e := range entries {
		// The cache and locks subdirs are not runners.
		if !e.IsDir() || e.Name() == "cache" || e.Name() == ".locks" {
			continue
		}
		dir := filepath.Join(root, e.Name())
		if marker, err := readInstallMarker(dir); err == nil {
			meta = append(meta, marker)
			continue
		}
		meta = append(meta, synthesizedMeta(cfg, p, dir))
	}
	return meta
}

// discoverInstallations enumerates installations for a scope. The
// returned warnings describe skipped registry entries (stale or
// unverified); they are printed but never cause management of the
// skipped installations.
//
// Scopes:
//   - "directory": the configured runners root only (no registry).
//   - "repo": the configured root plus registry installations whose
//     owning clone or owner/repo matches this invocation's repo.
//   - "pc": the configured root plus every valid registry installation
//     on this machine, regardless of repository.
func discoverInstallations(cfg *Config, p platform, scope string) ([]discoveredRunner, []string, error) {
	var out []discoveredRunner
	var warnings []string
	seen := map[string]bool{}
	add := func(m InstallationMeta, from string) {
		dir := filepath.Clean(m.InstallationDir)
		if dir == "" || seen[dir] {
			return
		}
		seen[dir] = true
		m.InstallationDir = dir
		out = append(out, discoveredRunner{InstallationMeta: m, From: from})
	}

	// 1) The configured runners root — always in scope.
	for _, m := range rootInstallations(cfg, p) {
		add(m, "root")
	}

	if scope == "directory" {
		sortDiscovered(out)
		return out, warnings, nil
	}

	// 2) The machine registry — only validated entries are managed.
	reg, err := loadRegistry()
	if err != nil {
		return nil, nil, err
	}
	for _, m := range reg.Installations {
		inst, warn, ok := validRegistryEntry(m)
		if !ok {
			warnings = append(warnings, warn)
			continue
		}
		if scope == "repo" && !repoMatches(cfg, inst) {
			continue
		}
		add(inst, "registry")
	}
	sortDiscovered(out)
	return out, warnings, nil
}

func sortDiscovered(out []discoveredRunner) {
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.RepoFullName != b.RepoFullName {
			return a.RepoFullName < b.RepoFullName
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.InstallationDir < b.InstallationDir
	})
}

// filterByNames keeps only installations whose runner name is in names.
// An empty names list keeps everything.
func filterByNames(found []discoveredRunner, names []string) []discoveredRunner {
	if len(names) == 0 {
		return found
	}
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	out := make([]discoveredRunner, 0, len(found))
	for _, d := range found {
		if want[d.Name] {
			out = append(out, d)
		}
	}
	return out
}

// scopedConfig returns a Config bound to one discovered installation, so
// the existing per-runner verbs (statusOne/startOne/...) operate on it
// unchanged. Owner/repo come from the marker when present.
func scopedConfig(cfg *Config, d discoveredRunner) *Config {
	sc := *cfg
	sc.RepoDir = d.RepoDir
	if sc.RepoDir == "" {
		sc.RepoDir = cfg.RepoDir
	}
	// The runners root for a discovered installation is the parent of
	// the installation dir. An absolute runners-dir is honored as-is by
	// Config.RunnersRoot.
	sc.RunnersDir = filepath.Dir(d.InstallationDir)
	if d.Owner != "" {
		sc.Owner = d.Owner
	}
	if d.Repo != "" {
		sc.Repo = d.Repo
	}
	sc.RunnerNames = nil
	return &sc
}
