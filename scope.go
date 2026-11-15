package main

import (
	"fmt"
	"io"
	"strings"
)

// This file resolves the --scope flag (and --org/--github) into either a
// local machine scope (directory|repo|pc) or a GitHub inventory scope
// (repo|org|owner, read-only, status only), and implements the
// shared batch runner used by the mutating verbs.
//
// Scope overview:
//
//	directory   (default) only the configured runners dir — the classic behavior
//	repo        every installation of the current repository, including those
//	            in other working directories (matched via the install registry)
//	pc          every runner-spawner-managed installation on this machine
//
// GitHub inventory (status only; always read-only):
//
//	repo        runners registered to one repository (--repo owner/repo, or
//	            --owner + --repo)
//	org         runners registered to an organization (--org, default: owner)
//	owner       aggregate inventory across every repository of an account or
//	            organization the token can see (--owner, default: config/git)

// resolveScope validates --scope/--org/--github for verb and decides
// the mode of this invocation. It returns localScope ("directory"|"repo"|"pc",
// empty in GitHub mode) or githubScope ("repo"|"org"|"owner", empty in local
// mode). When --repo is given as owner/repo, cfg is rewired to the split
// owner and repo, and the invocation is treated as GitHub inventory.
func resolveScope(verb, scopeArg, orgArg string, githubFlag bool, cfg *Config) (localScope, githubScope string, err error) {
	sc := strings.ToLower(strings.TrimSpace(scopeArg))
	orgSet := strings.TrimSpace(orgArg) != ""

	// --repo owner/repo (contains a slash) is always interpreted as a
	// GitHub inventory target and implies --scope repo + --github.
	if verb == "status" && strings.Contains(cfg.Repo, "/") && (sc == "" || sc == "repo" || sc == "directory") {
		idx := strings.Index(cfg.Repo, "/")
		o, r := strings.TrimSpace(cfg.Repo[:idx]), strings.TrimSpace(cfg.Repo[idx+1:])
		if o == "" || r == "" || strings.Contains(r, "/") {
			return "", "", usageErrf("invalid --repo %q (want owner/repo)", cfg.Repo)
		}
		cfg.Owner, cfg.Repo = o, r
		githubFlag = true
		sc = "repo" // empty scope + owner/repo form means GitHub inventory
	}

	switch verb {
	case "list", "start", "stop":
		if orgSet {
			return "", "", usageErrf("--org only applies to status --scope org")
		}
		if githubFlag {
			return "", "", usageErrf("--github only applies to status")
		}
		switch sc {
		case "", "directory":
			return "directory", "", nil
		case "repo", "pc":
			return sc, "", nil
		case "org", "owner":
			return "", "", usageErrf("--scope %s is a read-only GitHub inventory scope; use `runner-spawner status --scope %s ...`", sc, sc)
		default:
			return "", "", usageErrf("invalid --scope %q (want directory, repo or pc)", scopeArg)
		}

	case "create", "remove":
		if orgSet || githubFlag {
			return "", "", usageErrf("--org/--github are only supported by status")
		}
		if sc == "" || sc == "directory" {
			return "directory", "", nil
		}
		return "", "", usageErrf("--scope %q is not supported by %s (supported: directory)", scopeArg, verb)

	case "status":
		if orgSet && sc != "org" {
			return "", "", usageErrf("--org only applies to --scope org")
		}
		switch sc {
		case "", "directory":
			if githubFlag {
				return "", "", usageErrf("--github requires --scope repo, org or owner")
			}
			return "directory", "", nil
		case "pc":
			if githubFlag {
				return "", "", usageErrf("--github is not supported with --scope pc (use --scope repo, org or owner)")
			}
			return "pc", "", nil
		case "repo":
			if githubFlag {
				return "", "repo", nil
			}
			return "repo", "", nil
		case "org", "owner":
			return "", sc, nil
		default:
			return "", "", usageErrf("invalid --scope %q (want directory, repo, pc, org or owner)", scopeArg)
		}
	default:
		return "", "", usageErrf("unsupported verb %q", verb)
	}
}

// mutationTarget is one (config, runner) pair a verb operates on.
type mutationTarget struct {
	cfg  *Config
	name string
	repo string // owner/repo, or "" when unknown
}

// discoveredTargets resolves the installations for scope != directory
// into per-target Configs the classic verbs can run unmodified.
// Warnings describe skipped registry entries (stale/unverifiable).
func discoveredTargets(cfg *Config, p platform, scope string, nameFilter []string) ([]mutationTarget, []string, error) {
	found, warnings, err := discoverInstallations(cfg, p, scope)
	if err != nil {
		return nil, nil, err
	}
	found = filterByNames(found, nameFilter)
	targets := make([]mutationTarget, 0, len(found))
	for _, d := range found {
		full := d.RepoFullName
		targets = append(targets, mutationTarget{cfg: scopedConfig(cfg, d), name: d.Name, repo: full})
	}
	return targets, warnings, nil
}

// printWarnings surfaces skipped registry entries to stderr.
func printWarnings(warnings []string, stderr io.Writer) {
	for _, w := range warnings {
		fmt.Fprintf(stderr, "runner-spawner: warning: %s\n", w)
	}
}

// runBatch executes dispatch for every target, continuing after
// individual failures. JSON results are written once, at the end, with
// every per-target outcome included. When any target fails an
// aggregate error naming the failures is returned (exit code 1).
//
// When directoryScope is true the no-targets case is the classic
// config error (exit code 2); otherwise it is a note and success.
func runBatch(verb string, cfg *Config, scope string, targets []mutationTarget, filterProvided bool, directoryScope bool, dispatch func(t mutationTarget, stdout, stderr io.Writer) error, stdout, stderr io.Writer) error {
	if len(targets) == 0 {
		if directoryScope {
			return configErrf("no runner names given (pass --name or add runnerNames to %s)", DefaultConfigFile)
		}
		if filterProvided {
			return configErrf("no runner-spawner-managed installations matching --name found in scope %s", scope)
		}
		if cfg.Format == "json" {
			return writeMutationJSON(stdout, cfg, scope, nil)
		}
		fmt.Fprintf(stderr, "runner-spawner: no runner-spawner-managed installations found in scope %s\n", scope)
		return nil
	}

	var results []mutationResult
	var failures []string
	for _, t := range targets {
		err := dispatch(t, stdout, stderr)
		res := mutationResult{Name: t.name, Repo: t.repo, OK: err == nil}
		if err != nil {
			res.Error = err.Error()
			label := t.name
			if t.repo != "" {
				label = fmt.Sprintf("%s (%s)", t.name, t.repo)
			}
			failures = append(failures, fmt.Sprintf("%s: %v", label, err))
		}
		if cfg.Format == "json" {
			results = append(results, res)
		}
	}
	if cfg.Format == "json" {
		if err := writeMutationJSON(stdout, cfg, scope, results); err != nil {
			return err
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("%d of %d %s runner(s) failed: %s", len(failures), len(targets), verb, strings.Join(failures, "; "))
	}
	return nil
}
