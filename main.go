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
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
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
	if ce, ok := err.(*codedError); ok {
		return ce.code
	}
	return 1
}

func configErrf(format string, args ...any) error {
	return &codedError{code: 2, msg: fmt.Sprintf(format, args...)}
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
	for _, n := range strings.Split(s, ",") {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		*f.names = append(*f.names, n)
	}
	return nil
}

func run(args []string, stdout, stderr io.Writer) error {
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
	default:
		fmt.Fprintf(stderr, "unknown subcommand %q\n\n", args[0])
		printUsage(stderr)
		return usageErrf("unknown subcommand %q", args[0])
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
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Global flags (accepted by every subcommand):")
	fmt.Fprintln(w, "  --config PATH        path to .runner-spawner.json (auto-discovered by default)")
	fmt.Fprintln(w, "  --repo-dir DIR       target repository directory (default: .)")
	fmt.Fprintln(w, "  --owner OWNER        GitHub owner (default: .runner-spawner.json / git remote)")
	fmt.Fprintln(w, "  --repo REPO          GitHub repository (default: .runner-spawner.json / git remote)")
	fmt.Fprintln(w, "  --name NAME          runner name (repeatable/comma-separated; default: config names)")
	fmt.Fprintln(w, "  --runners-dir DIR    runners root (default: .runners under --repo-dir)")
	fmt.Fprintln(w, "  --runner-group NAME  runner group (default: Default)")
	fmt.Fprintln(w, "  --runner-version V   actions/runner version (default: 2.338.2)")
	fmt.Fprintln(w, "  --archive-cache-dir DIR  archive cache (default: <runners-dir>/cache)")
	fmt.Fprintln(w, "  --labels A,B,...     runner labels (default: auto from platform+hostname)")
	fmt.Fprintln(w, "  --token TOKEN        registration token (default: RUNNER_SPAWNER_TOKEN / GH_TOKEN / gh)")
	fmt.Fprintln(w, "  --dry-run            print what would happen, change nothing")
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
	fmt.Fprintln(w, "See README.md for .runner-spawner.json and examples.")
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
	formatFlag := fs.String("format", "", "output format: text, json (default: text)")
	quietFlag := fs.Bool("quiet", false, "suppress status logs on stderr")
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
		return usageErrf("%v", err)
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
			fmt.Fprintln(stderr, "runner-spawner:", err)
			return configErrf("%v", err)
		}
	} else if explicitConfig != "" {
		return configErrf("config file %q not found", explicitConfig)
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
		fmt.Fprintln(stderr, "runner-spawner:", err)
		return configErrf("%v", err)
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
	if *formatFlag != "" {
		cfg.Format = *formatFlag
	}
	if *quietFlag {
		cfg.Quiet = true
	}

	switch cfg.Format {
	case "text", "json":
	default:
		return configErrf("invalid --format %q (want text or json)", cfg.Format)
	}

	p := currentPlatform()
	names := cfg.ResolveNames(flagNames)

	needRepo := verb == "create" || verb == "remove"
	if err := cfg.Validate(needRepo, names); err != nil {
		return configErrf("%v", err)
	}

	switch verb {
	case "list":
		all, err := listNames(cfg)
		if err != nil {
			return err
		}
		if len(all) == 0 {
			return fmt.Errorf("no runners found (pass --name, add runnerNames to %s, or check --runners-dir)", DefaultConfigFile)
		}
		var out []runnerStatus
		for _, n := range all {
			st, err := statusOne(cfg, p, n)
			if err != nil {
				return err
			}
			out = append(out, st)
		}
		return writeStatus(stdout, cfg, out)
	case "status":
		if len(names) == 0 {
			return configErrf("no runner names given (pass --name or add runnerNames to %s)", DefaultConfigFile)
		}
		var out []runnerStatus
		for _, n := range names {
			st, err := statusOne(cfg, p, n)
			if err != nil {
				return err
			}
			out = append(out, st)
		}
		return writeStatus(stdout, cfg, out)
	default:
		if len(names) == 0 {
			return configErrf("no runner names given (pass --name or add runnerNames to %s)", DefaultConfigFile)
		}
		for _, n := range names {
			var err error
			switch verb {
			case "create":
				err = createOne(cfg, p, n, stdout, stderr)
			case "remove":
				err = removeOne(cfg, p, n, stderr)
			case "start":
				err = startOne(cfg, p, n, stderr)
			case "stop":
				err = stopOne(cfg, p, n, stderr)
			}
			if err != nil {
				return err
			}
		}
		return nil
	}
}

func writeStatus(w io.Writer, cfg *Config, out []runnerStatus) error {
	if cfg.Format == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{
			"repo":    cfg.Owner + "/" + cfg.Repo,
			"runners": out,
		})
	}
	for _, st := range out {
		fmt.Fprintf(w, "%s  %s  configured=%v  service=%s  %s\n",
			st.Name, st.Display, st.Configured, st.ServiceState, st.Dir)
	}
	return nil
}
