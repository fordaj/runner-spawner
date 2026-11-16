// Command runner-spawner installs and manages repository-scoped
// self-hosted GitHub Actions runners for a locally cloned repository.
//
// It is the Go replacement for the old _create_runner.sh /
// _remove_runner.sh / _start_runner.sh / _stop_runner.sh /
// create_all_runners.sh family: the target repo, runner names, and
// runners dir come from .runner-spawner.json (or flags/env) instead of
// values baked into Utilities/.env, the actions-runner archive is
// downloaded once and cached (no vendored Tools/actions-runner path,
// no 7za dependency), and macOS/Windows service handling is
// status-aware (start/stop are idempotent, remove always stops first).
//
// Configuration precedence (lowest to highest): built-in defaults, git
// remote detection (owner/repo only), .runner-spawner.json,
// RUNNER_SPAWNER_* env vars, CLI flags.
//
// Exit codes: 0 on success, 1 on a runtime error, 2 on bad flags/config.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	// run prints errors to stderr (errors are never suppressed by --quiet;
	// --quiet only suppresses status logs), so main only maps the error to
	// an exit code.
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		os.Exit(exitCode(err))
	}
}

// codedError carries an exit code.
type codedError struct {
	code int
	msg  string
}

func (e *codedError) Error() string { return e.msg }

func exitCode(err error) int {
	var ce *codedError
	if errors.As(err, &ce) {
		return ce.code
	}
	return 1
}

// reportedError marks an error whose message was already printed to stderr
// (e.g. by the flag package or an explicit usage print), so the central
// error print in run does not duplicate it.
type reportedError struct {
	err error
}

func (e *reportedError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *reportedError) Unwrap() error { return e.err }

func markReported(err error) error { return &reportedError{err: err} }

func isReported(err error) bool {
	var re *reportedError
	return errors.As(err, &re)
}

func usageErrf(format string, args ...any) error {
	return &codedError{code: 2, msg: fmt.Sprintf(format, args...)}
}

type nameFlag struct {
	names *[]string
}

func (f nameFlag) String() string {
	if f.names == nil {
		return ""
	}
	return strings.Join(*f.names, ",")
}

func (f nameFlag) Set(s string) error {
	*f.names = append(*f.names, ParseList(s)...)
	return nil
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	err := runInner(args, stdin, stdout, stderr)
	if err != nil && !isReported(err) {
		fmt.Fprintln(stderr, "runner-spawner:", err)
	}
	return err
}

func runInner(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printUsage(stderr)
		return &codedError{code: 2, msg: "missing subcommand"}
	}
	switch args[0] {
	case "--help", "-h", "help":
		printUsage(stdout)
		return nil
	case "--version", "-version", "version":
		fmt.Fprintln(stdout, "runner-spawner "+toolVersion)
		return nil
	case "create", "remove", "start", "stop", "status", "list":
		return runVerb(args[0], args[1:], stdout, stderr)
	case "init":
		return runInit(args[1:], stdin, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown subcommand %q\n\n", args[0])
		printUsage(stderr)
		return markReported(usageErrf("unknown subcommand %q", args[0]))
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "runner-spawner - install and manage repository-scoped self-hosted runners")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Usage: runner-spawner <subcommand> [flags]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Subcommands:")
	fmt.Fprintln(w, "  create   Install (configure + service) one or more runners")
	fmt.Fprintln(w, "  remove   Stop, uninstall, deregister, and delete runners")
	fmt.Fprintln(w, "  start    Start runner service(s)")
	fmt.Fprintln(w, "  stop     Stop runner service(s)")
	fmt.Fprintln(w, "  status   Show per-runner service state")
	fmt.Fprintln(w, "  list     List runners (config names + on-disk dirs)")
	fmt.Fprintln(w, "  init     Write a .runner-spawner.json for this repository")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Scopes (--scope; list, status, start, stop):")
	fmt.Fprintln(w, "  directory  (default) only the runners in the configured --runners-dir")
	fmt.Fprintln(w, "  repo       all runners of the current repository, incl. other working directories")
	fmt.Fprintln(w, "  pc         all runner-spawner-managed installations on this machine")
	fmt.Fprintln(w, "  org        (status only, read-only) GitHub runners registered to an organization")
	fmt.Fprintln(w, "  owner      (status only, read-only) GitHub runners across repos of an account/org")
	fmt.Fprintln(w, "GitHub inventory via `status --scope repo|org|owner` is always read-only.")
	fmt.Fprintln(w, "Examples: runner-spawner list --scope pc")
	fmt.Fprintln(w, "          runner-spawner status --scope org --org fordaj")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Global flags (accepted by every subcommand):")
	fmt.Fprintln(w, "  --config PATH        path to .runner-spawner.json (auto-discovered by default)")
	fmt.Fprintln(w, "  --repo-dir DIR       target repository directory (default: .)")
	fmt.Fprintln(w, "  --owner OWNER        GitHub owner (default: .runner-spawner.json / git remote)")
	fmt.Fprintln(w, "  --repo REPO          GitHub repository (default: .runner-spawner.json / git remote)")
	fmt.Fprintln(w, "  --name NAME          runner name (repeatable/comma-separated; default: config names)")
	fmt.Fprintln(w, "  --scope SCOPE        list,status,start,stop: directory|repo|pc (default: directory);")
	fmt.Fprintln(w, "                       status also: org|owner (GitHub inventory, read-only)")
	fmt.Fprintln(w, "  --org ORG            GitHub organization (status --scope org)")
	fmt.Fprintln(w, "  --github             status: force GitHub inventory mode (status --scope repo)")
	fmt.Fprintln(w, "  --runners-dir DIR    runners root (default: .runners under --repo-dir)")
	fmt.Fprintln(w, "  --runner-group NAME  runner group (default: Default)")
	fmt.Fprintln(w, "  --runner-version V   actions/runner version (default: 2.338.2)")
	fmt.Fprintln(w, "  --archive-cache-dir DIR  archive cache (default: <runners-dir>/cache)")
	fmt.Fprintln(w, "  --labels A,B,...     runner labels (default: auto from platform+hostname)")
	fmt.Fprintln(w, "  --token TOKEN        registration token (default: RUNNER_SPAWNER_TOKEN / GH_TOKEN / gh)")
	fmt.Fprintln(w, "  --dry-run            print what would happen, change nothing")
	fmt.Fprintln(w, "  --local-only         remove: skip GitHub deregistration (local cleanup only)")
	fmt.Fprintln(w, "  --format text|json   output format (default: text)")
	fmt.Fprintln(w, "  --quiet              suppress status logs on stderr")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Windows only: --windows-username USER --windows-password PASS")
	fmt.Fprintln(w, "  (prefer RUNNER_SPAWNER_WINDOWS_USERNAME / RUNNER_SPAWNER_WINDOWS_PASSWORD)")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Environment: RUNNER_SPAWNER_CONFIG, RUNNER_SPAWNER_REPO_DIR,")
	fmt.Fprintln(w, "  RUNNER_SPAWNER_OWNER, RUNNER_SPAWNER_REPO, RUNNER_SPAWNER_RUNNER_NAMES,")
	fmt.Fprintln(w, "  RUNNER_SPAWNER_RUNNERS_DIR, RUNNER_SPAWNER_RUNNER_GROUP,")
	fmt.Fprintln(w, "  RUNNER_SPAWNER_RUNNER_VERSION, RUNNER_SPAWNER_ARCHIVE_CACHE_DIR,")
	fmt.Fprintln(w, "  RUNNER_SPAWNER_LABELS, RUNNER_SPAWNER_WINDOWS_USERNAME,")
	fmt.Fprintln(w, "  RUNNER_SPAWNER_WINDOWS_PASSWORD, RUNNER_SPAWNER_TOKEN,")
	fmt.Fprintln(w, "  RUNNER_SPAWNER_DRY_RUN, RUNNER_SPAWNER_FORMAT, RUNNER_SPAWNER_QUIET.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Supported platforms: macOS and Windows. Linux is not yet supported.")
	fmt.Fprintln(w, "See README.md for .runner-spawner.json (or run `runner-spawner init`).")
}

func runVerb(verb string, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("runner-spawner "+verb, flag.ContinueOnError)
	fs.SetOutput(stderr)

	cfg := DefaultConfig()
	configPath := fs.String("config", "", "path to .runner-spawner.json (auto-discovered by default)")
	repoDirFlag := fs.String("repo-dir", "", "target repository directory (default: .)")
	ownerFlag := fs.String("owner", "", "GitHub owner")
	repoFlag := fs.String("repo", "", "GitHub repository")
	runnersDirFlag := fs.String("runners-dir", "", "runners root (default: .runners under --repo-dir)")
	runnerGroupFlag := fs.String("runner-group", "", "runner group (default: Default)")
	runnerVersionFlag := fs.String("runner-version", "", "actions/runner version (default: 2.338.2)")
	cacheFlag := fs.String("archive-cache-dir", "", "archive cache dir (default: <runners-dir>/cache)")
	labelsFlag := fs.String("labels", "", "runner labels, comma-separated (default: auto)")
	tokenFlag := fs.String("token", "", "registration token (default: RUNNER_SPAWNER_TOKEN / GH_TOKEN / gh)")
	windowsUserFlag := fs.String("windows-username", "", "Windows service account (required on Windows)")
	windowsPassFlag := fs.String("windows-password", "", "Windows service account password (required on Windows)")
	dryRunFlag := fs.Bool("dry-run", false, "print what would happen, change nothing")
	localOnlyFlag := fs.Bool("local-only", false, "remove: skip GitHub deregistration (local cleanup only)")
	formatFlag := fs.String("format", "", "output format: text, json (default: text)")
	quietFlag := fs.Bool("quiet", false, "suppress status logs on stderr")
	scopeFlag := fs.String("scope", "", "scope: directory|repo|pc (default: directory); status also: org|owner (GitHub inventory, read-only)")
	orgFlag := fs.String("org", "", "GitHub organization for status --scope org (default: owner from config/env/git remote)")
	githubFlag := fs.Bool("github", false, "status: query GitHub instead of local service state")
	showVersion := fs.Bool("version", false, "print tool version and exit")
	var flagNames []string
	fs.Var(nameFlag{names: &flagNames}, "name", "runner name (repeatable/comma-separated)")

	fs.Usage = func() {
		fmt.Fprintf(stderr, "runner-spawner %s - install and manage repository-scoped self-hosted runners\n\n", verb)
		fmt.Fprintln(stderr, "Flags:")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		// The flag package already printed the error and usage to stderr.
		return markReported(usageErrf("%v", err))
	}
	if fs.NArg() > 0 {
		return usageErrf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	if *showVersion {
		fmt.Fprintln(stdout, "runner-spawner "+toolVersion)
		return nil
	}

	// Preliminary repo dir for config discovery: flag > env > default.
	prelimRepoDir := cfg.RepoDir
	if v := os.Getenv(EnvRepoDir); v != "" {
		prelimRepoDir = v
	}
	if *repoDirFlag != "" {
		prelimRepoDir = *repoDirFlag
	}
	cfg.RepoDir = prelimRepoDir

	// Explicit config: --config > RUNNER_SPAWNER_CONFIG; else auto-discover.
	explicitConfig := os.Getenv(EnvConfig)
	if *configPath != "" {
		explicitConfig = *configPath
	}
	configFile := explicitConfig
	if configFile == "" {
		configFile = FindConfigFile(prelimRepoDir)
	}
	if configFile != "" {
		if err := cfg.LoadFile(configFile); err != nil {
			return usageErrf("%v", err)
		}
	} else if explicitConfig != "" {
		return usageErrf("config file %q not found", explicitConfig)
	}

	// Git remote fills owner/repo only while still unknown (lowest source).
	if cfg.Owner == "" || cfg.Repo == "" {
		if o, r, ok := detectGitRemoteOwnerRepo(prelimRepoDir); ok {
			if cfg.Owner == "" {
				cfg.Owner = o
			}
			if cfg.Repo == "" {
				cfg.Repo = r
			}
		}
	}

	// Env layer.
	if err := cfg.ApplyEnv(nil); err != nil {
		return usageErrf("%v", err)
	}

	// Flag layer (explicit only; unset flags must not clobber env/file).
	if *repoDirFlag != "" {
		cfg.RepoDir = *repoDirFlag
	}
	if *ownerFlag != "" {
		cfg.Owner = *ownerFlag
	}
	if *repoFlag != "" {
		cfg.Repo = *repoFlag
	}
	if *runnersDirFlag != "" {
		cfg.RunnersDir = *runnersDirFlag
	}
	if *runnerGroupFlag != "" {
		cfg.RunnerGroup = *runnerGroupFlag
	}
	if *runnerVersionFlag != "" {
		cfg.RunnerVersion = *runnerVersionFlag
	}
	if *cacheFlag != "" {
		cfg.ArchiveCacheDir = *cacheFlag
	}
	if *labelsFlag != "" {
		cfg.Labels = ParseList(*labelsFlag)
	}
	if *tokenFlag != "" {
		cfg.Token = *tokenFlag
	}
	if *windowsUserFlag != "" {
		cfg.WindowsUsername = *windowsUserFlag
	}
	if *windowsPassFlag != "" {
		cfg.WindowsPassword = *windowsPassFlag
	}
	if *dryRunFlag {
		cfg.DryRun = true
	}
	if *localOnlyFlag {
		cfg.LocalOnly = true
	}
	if *formatFlag != "" {
		cfg.Format = *formatFlag
	}
	if *quietFlag {
		cfg.Quiet = true
	}

	// Resolve the scope and mode of this invocation before doing any
	// work. In GitHub inventory mode cfg.Repo may be rewritten from
	// owner/repo form. Explicit --name flags filter scoped operations;
	// config runnerNames apply to the default directory scope only.
	localScope, ghScope, scopeErr := resolveScope(verb, *scopeFlag, *orgFlag, *githubFlag, cfg)
	if scopeErr != nil {
		return scopeErr
	}

	// Format-only validation, then resolve names.
	switch cfg.Format {
	case "text", "json":
	default:
		return usageErrf("invalid --format %q (want text or json)", cfg.Format)
	}

	p := currentPlatform()
	names := cfg.ResolveNames(flagNames)
	if localScope != "directory" {
		names = flagNames
	}

	needRepo := verb == "create" || verb == "remove"
	if err := cfg.Validate(needRepo); err != nil {
		return usageErrf("%v", err)
	}

	if ghScope != "" {
		return runInventory(cfg, ghScope, *orgFlag, stdout, stderr)
	}

	repoFull := repoFullName(cfg)
	switch verb {
	case "list":
		if localScope == "directory" {
			all, err := listNames(cfg)
			if err != nil {
				return err
			}
			if len(all) == 0 {
				return fmt.Errorf("no runners found (pass --name, add runnerNames to %s, or check --runners-dir)", DefaultConfigFile)
			}
			out, err := statusForTargets(p, simpleTargets(cfg, all, ""))
			if err != nil {
				return err
			}
			return writeStatus(stdout, cfg, "directory", out)
		}
		targets, warnings, err := discoveredTargets(cfg, p, localScope, nil)
		printWarnings(warnings, stderr)
		if err != nil {
			return err
		}
		out, err := statusForTargets(p, targets)
		if err != nil {
			return err
		}
		if len(out) == 0 {
			return writeEmptyScopeStatus(stdout, stderr, cfg, localScope)
		}
		return writeStatus(stdout, cfg, localScope, out)
	case "status":
		if localScope == "directory" {
			if len(names) == 0 {
				return usageErrf("no runner names given (pass --name or add runnerNames to %s)", DefaultConfigFile)
			}
			out, err := statusForTargets(p, simpleTargets(cfg, names, ""))
			if err != nil {
				return err
			}
			return writeStatus(stdout, cfg, "directory", out)
		}
		targets, warnings, err := discoveredTargets(cfg, p, localScope, flagNames)
		printWarnings(warnings, stderr)
		if err != nil {
			return err
		}
		if len(targets) == 0 {
			if len(flagNames) > 0 {
				return usageErrf("no runner-spawner-managed installations named %s found in scope %s", strings.Join(flagNames, ", "), localScope)
			}
			return writeEmptyScopeStatus(stdout, stderr, cfg, localScope)
		}
		out, err := statusForTargets(p, targets)
		if err != nil {
			return err
		}
		return writeStatus(stdout, cfg, localScope, out)
	default: // create / remove / start / stop
		directoryScope := localScope == "directory"
		var targets []mutationTarget
		if directoryScope {
			targets = simpleTargets(cfg, names, repoFull)
		} else {
			var warnings []string
			var err error
			targets, warnings, err = discoveredTargets(cfg, p, localScope, flagNames)
			printWarnings(warnings, stderr)
			if err != nil {
				return err
			}
		}
		dispatch := func(t mutationTarget, stdout, stderr io.Writer) error {
			switch verb {
			case "create":
				return createOne(t.cfg, p, t.name, stdout, stderr)
			case "remove":
				return removeOne(t.cfg, p, t.name, stdout, stderr)
			case "start":
				return startOne(t.cfg, p, t.name, stdout, stderr)
			case "stop":
				return stopOne(t.cfg, p, t.name, stdout, stderr)
			}
			return nil
		}
		return runBatch(verb, cfg, localScope, targets, len(flagNames) > 0, directoryScope, dispatch, stdout, stderr)
	}
}

// simpleTargets builds directory-scope mutation targets bound to cfg.
func simpleTargets(cfg *Config, names []string, repo string) []mutationTarget {
	targets := make([]mutationTarget, 0, len(names))
	for _, n := range names {
		targets = append(targets, mutationTarget{cfg: cfg, name: n, repo: repo})
	}
	return targets
}

// statusForTargets inspects every target without mutating anything.
func statusForTargets(p platform, targets []mutationTarget) ([]runnerStatus, error) {
	out := make([]runnerStatus, 0, len(targets))
	for _, t := range targets {
		st, err := statusOne(t.cfg, p, t.name)
		if err != nil {
			return nil, err
		}
		if t.repo != "" {
			st.Repo = t.repo
		}
		out = append(out, st)
	}
	return out, nil
}

// writeEmptyScopeStatus reports "no runner-spawner-managed installations in
// scope": an empty JSON document in json mode, an explanatory note otherwise.
func writeEmptyScopeStatus(stdout, stderr io.Writer, cfg *Config, scope string) error {
	if cfg.Format == "json" {
		return writeStatus(stdout, cfg, scope, nil)
	}
	fmt.Fprintf(stderr, "runner-spawner: no runner-spawner-managed installations found in scope %s\n", scope)
	return nil
}

// repoFullName renders cfg's owner/repo as "owner/repo", or "" when unknown.
func repoFullName(cfg *Config) string {
	if cfg.Owner == "" || cfg.Repo == "" {
		return ""
	}
	return cfg.Owner + "/" + cfg.Repo
}

// mutationResult is one entry of the --format json output of the mutating
// verbs (create/remove/start/stop).
type mutationResult struct {
	Name  string `json:"name"`
	Repo  string `json:"repo,omitempty"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// writeJSONIndent encodes v as indented JSON with a trailing newline.
func writeJSONIndent(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func writeMutationJSON(w io.Writer, cfg *Config, scope string, results []mutationResult) error {
	if results == nil {
		results = []mutationResult{}
	}
	if scope == "" {
		scope = "directory"
	}
	out := map[string]any{
		"scope":   scope,
		"results": results,
	}
	// The classic "repo" key stays for directory scope (compatibility).
	if scope == "directory" {
		out["repo"] = cfg.Owner + "/" + cfg.Repo
	}
	return writeJSONIndent(w, out)
}

func writeStatus(w io.Writer, cfg *Config, scope string, out []runnerStatus) error {
	if out == nil {
		out = []runnerStatus{}
	}
	if cfg.Format == "json" {
		m := map[string]any{
			"scope":   scope,
			"runners": out,
		}
		if scope == "directory" {
			m["repo"] = cfg.Owner + "/" + cfg.Repo
		}
		return writeJSONIndent(w, m)
	}
	for _, st := range out {
		fmt.Fprintf(w, "%s  %s", st.Name, st.Display)
		if st.Repo != "" {
			fmt.Fprintf(w, "  repo=%s", st.Repo)
		}
		fmt.Fprintf(w, "  configured=%v  service=%s  %s",
			st.Configured, st.ServiceState, st.Dir)
		if st.Hint != "" {
			fmt.Fprintf(w, "  hint: %s", st.Hint)
		}
		fmt.Fprintln(w)
	}
	return nil
}
