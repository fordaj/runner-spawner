package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config holds every tunable of runner-spawner.
//
// Precedence, lowest to highest: built-in defaults, git remote detection
// (owner/repo only), .runner-spawner.json, RUNNER_SPAWNER_* env vars, CLI flags.
//
// Config file location: <repoDir>/.runner-spawner.json (auto-discovered),
// unless --config / RUNNER_SPAWNER_CONFIG points elsewhere. Copy
// runner-spawner.example.json to get started.
type Config struct {
	ConfigPath string `json:"-"`
	RepoDir    string `json:"repoDir"`
	Owner      string `json:"owner"`
	Repo       string `json:"repo"`

	RunnerNames []string `json:"runnerNames"`
	RunnersDir  string   `json:"runnersDir"`
	RunnerGroup string   `json:"runnerGroup"`
	// RunnerVersion is the actions/runner release version without the "v"
	// prefix (e.g. "2.338.2"). Empty means the built-in default.
	RunnerVersion string `json:"runnerVersion"`
	// ArchiveCacheDir caches downloaded actions-runner archives. Empty
	// means <runnersDir>/cache.
	ArchiveCacheDir string `json:"archiveCacheDir"`
	// Labels optionally overrides the auto-detected platform+host labels.
	Labels []string `json:"labels"`

	// WindowsUsername is the service account the Windows runner service
	// runs as. Prefer the RUNNER_SPAWNER_WINDOWS_USERNAME env var over
	// committing it to .runner-spawner.json.
	WindowsUsername string `json:"windowsUsername"`
	WindowsPassword string `json:"-"`

	// Flags-only knobs, not serialised to .runner-spawner.json.
	DryRun bool   `json:"-"`
	Token  string `json:"-"`
	Format string `json:"-"`
	Quiet  bool   `json:"-"`
}

// DefaultConfig returns the built-in defaults.
func DefaultConfig() *Config {
	return &Config{
		RepoDir:       ".",
		RunnersDir:    ".runners",
		RunnerGroup:   "Default",
		RunnerVersion: "2.338.2",
		DryRun:        false,
		Format:        "text",
	}
}

// DefaultConfigFile is the name of the per-project config file.
const DefaultConfigFile = ".runner-spawner.json"

// Env vars honoured by ApplyEnv.
const (
	EnvConfig          = "RUNNER_SPAWNER_CONFIG"
	EnvRepoDir         = "RUNNER_SPAWNER_REPO_DIR"
	EnvOwner           = "RUNNER_SPAWNER_OWNER"
	EnvRepo            = "RUNNER_SPAWNER_REPO"
	EnvRunnerNames     = "RUNNER_SPAWNER_RUNNER_NAMES"
	EnvRunnersDir      = "RUNNER_SPAWNER_RUNNERS_DIR"
	EnvRunnerGroup     = "RUNNER_SPAWNER_RUNNER_GROUP"
	EnvRunnerVersion   = "RUNNER_SPAWNER_RUNNER_VERSION"
	EnvArchiveCacheDir = "RUNNER_SPAWNER_ARCHIVE_CACHE_DIR"
	EnvLabels          = "RUNNER_SPAWNER_LABELS"
	EnvWindowsUsername = "RUNNER_SPAWNER_WINDOWS_USERNAME"
	EnvWindowsPassword = "RUNNER_SPAWNER_WINDOWS_PASSWORD"
	EnvToken           = "RUNNER_SPAWNER_TOKEN"
	EnvDryRun          = "RUNNER_SPAWNER_DRY_RUN"
	EnvFormat          = "RUNNER_SPAWNER_FORMAT"
	EnvQuiet           = "RUNNER_SPAWNER_QUIET"
)

// FindConfigFile locates a .runner-spawner.json for the repo dir.
// It checks repoDir/.runner-spawner.json then the current working directory's.
// Returns "" when no file exists.
func FindConfigFile(repoDir string) string {
	for _, dir := range []string{repoDir, "."} {
		if dir == "" {
			continue
		}
		p := filepath.Join(dir, DefaultConfigFile)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// LoadFile parses a .runner-spawner.json file into a Config overlaid on dst.
// Unknown keys are rejected so typos in the committed file fail fast instead
// of being silently ignored.
func (c *Config) LoadFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	c.ConfigPath = path

	var raw struct {
		RepoDir         *string  `json:"repoDir"`
		Owner           *string  `json:"owner"`
		Repo            *string  `json:"repo"`
		RunnerNames     []string `json:"runnerNames"`
		RunnersDir      *string  `json:"runnersDir"`
		RunnerGroup     *string  `json:"runnerGroup"`
		RunnerVersion   *string  `json:"runnerVersion"`
		ArchiveCacheDir *string  `json:"archiveCacheDir"`
		Labels          []string `json:"labels"`
		WindowsUsername *string  `json:"windowsUsername"`
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&raw); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if dec.More() {
		return fmt.Errorf("%s: trailing data after JSON object", path)
	}
	if raw.RepoDir != nil {
		c.RepoDir = *raw.RepoDir
	}
	if raw.Owner != nil {
		c.Owner = *raw.Owner
	}
	if raw.Repo != nil {
		c.Repo = *raw.Repo
	}
	if raw.RunnerNames != nil {
		c.RunnerNames = raw.RunnerNames
	}
	if raw.RunnersDir != nil {
		c.RunnersDir = *raw.RunnersDir
	}
	if raw.RunnerGroup != nil {
		c.RunnerGroup = *raw.RunnerGroup
	}
	if raw.RunnerVersion != nil {
		c.RunnerVersion = *raw.RunnerVersion
	}
	if raw.ArchiveCacheDir != nil {
		c.ArchiveCacheDir = *raw.ArchiveCacheDir
	}
	if raw.Labels != nil {
		c.Labels = raw.Labels
	}
	if raw.WindowsUsername != nil {
		c.WindowsUsername = *raw.WindowsUsername
	}
	return nil
}

// ParseList parses a comma-separated list. Empty string yields an empty
// (non-nil) slice. Whitespace around entries is trimmed.
func ParseList(s string) []string {
	out := []string{}
	s = strings.TrimSpace(s)
	if s == "" {
		return out
	}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

// ParseBool parses boolean env values.
func ParseBool(s string) (bool, error) {
	return strconv.ParseBool(strings.TrimSpace(s))
}

// ApplyEnv overlays RUNNER_SPAWNER_* variables. getenv defaults to os.Getenv
// so tests can inject an environment. EnvConfig is consumed by main before
// ApplyEnv (it selects the file) and EnvToken is consumed by the token
// resolver, so they are not applied here.
func (c *Config) ApplyEnv(getenv func(string) string) error {
	if getenv == nil {
		getenv = os.Getenv
	}
	if v := getenv(EnvRepoDir); v != "" {
		c.RepoDir = v
	}
	if v := getenv(EnvOwner); v != "" {
		c.Owner = v
	}
	if v := getenv(EnvRepo); v != "" {
		c.Repo = v
	}
	if v := getenv(EnvRunnerNames); v != "" {
		c.RunnerNames = ParseList(v)
	}
	if v := getenv(EnvRunnersDir); v != "" {
		c.RunnersDir = v
	}
	if v := getenv(EnvRunnerGroup); v != "" {
		c.RunnerGroup = v
	}
	if v := getenv(EnvRunnerVersion); v != "" {
		c.RunnerVersion = v
	}
	if v := getenv(EnvArchiveCacheDir); v != "" {
		c.ArchiveCacheDir = v
	}
	if v := getenv(EnvLabels); v != "" {
		c.Labels = ParseList(v)
	}
	if v := getenv(EnvWindowsUsername); v != "" {
		c.WindowsUsername = v
	}
	if v := getenv(EnvWindowsPassword); v != "" {
		c.WindowsPassword = v
	}
	if v := getenv(EnvDryRun); v != "" {
		b, err := ParseBool(v)
		if err != nil {
			return fmt.Errorf("%s: invalid boolean %q", EnvDryRun, v)
		}
		c.DryRun = b
	}
	if v := getenv(EnvFormat); v != "" {
		c.Format = v
	}
	if v := getenv(EnvQuiet); v != "" {
		b, err := ParseBool(v)
		if err != nil {
			return fmt.Errorf("%s: invalid boolean %q", EnvQuiet, v)
		}
		c.Quiet = b
	}
	return nil
}

// Validate checks the merged config. nameArg is the set of names given
// directly to the subcommand; an empty result with needNames=true is an
// error. Owner/Repo are required for create/remove but not for
// start/stop/status/list which only touch local directories.
func (c *Config) Validate(needRepo bool, nameArg []string) error {
	if c.RepoDir == "" {
		return fmt.Errorf("repo dir must not be empty")
	}
	if c.RunnersDir == "" {
		return fmt.Errorf("runners dir must not be empty")
	}
	if c.RunnerGroup == "" {
		return fmt.Errorf("runner group must not be empty")
	}
	if c.RunnerVersion == "" {
		return fmt.Errorf("runner version must not be empty")
	}
	if needRepo {
		if c.Owner == "" || c.Repo == "" {
			return fmt.Errorf("owner/repo unknown: pass --owner/--repo, set RUNNER_SPAWNER_OWNER/RUNNER_SPAWNER_REPO, add owner/repo to %s, or run inside a git clone with a GitHub origin remote", DefaultConfigFile)
		}
	}
	_ = nameArg
	return nil
}

// ResolveNames picks the effective runner names: explicit --name values
// win over config/env; if neither provides any, callers that require a
// name get a "no names given" error from their own validation.
func (c *Config) ResolveNames(flagNames []string) []string {
	if len(flagNames) > 0 {
		return flagNames
	}
	if len(c.RunnerNames) > 0 {
		return c.RunnerNames
	}
	return nil
}

// AbsRepoDir returns the absolute repo dir.
func (c *Config) AbsRepoDir() (string, error) {
	dir := c.RepoDir
	if dir == "" {
		dir = "."
	}
	return filepath.Abs(dir)
}

// RunnersRoot returns the absolute runners root dir.
func (c *Config) RunnersRoot() (string, error) {
	abs, err := c.AbsRepoDir()
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(c.RunnersDir) {
		return filepath.Clean(c.RunnersDir), nil
	}
	return filepath.Join(abs, c.RunnersDir), nil
}

// ArchiveCache returns the absolute archive cache dir.
func (c *Config) ArchiveCache() (string, error) {
	if c.ArchiveCacheDir != "" {
		if filepath.IsAbs(c.ArchiveCacheDir) {
			return filepath.Clean(c.ArchiveCacheDir), nil
		}
		abs, err := c.AbsRepoDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(abs, c.ArchiveCacheDir), nil
	}
	root, err := c.RunnersRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "cache"), nil
}
