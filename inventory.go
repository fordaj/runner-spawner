package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// This file implements read-only GitHub runner inventory for
// `runner-spawner status --scope repo|org|owner`.
//
// It talks to GitHub exclusively through the `gh` CLI, honoring gh's
// existing authentication (GH_TOKEN / GITHUB_TOKEN / gh auth login) and
// permission model. Nothing here can start, stop, create or remove a
// runner — every call is a GET.
//
// GitHub exposes runner listings at:
//   - repos/{owner}/{repo}/actions/runners   (repository-scoped)
//   - orgs/{org}/actions/runners             (organization-scoped)
//
// There is no universal endpoint for "all runners of an account", so
// owner-wide inventory enumerates the repositories visible to the
// authenticated user (users/{owner}/repos?type=owner, or
// orgs/{org}/repos?type=all for organizations) and queries each
// repository's endpoint. Inaccessible repositories or organizations are
// reported as partial failures instead of aborting the whole
// inventory; results already gathered are still printed.

// inventoryRunner is one runner as reported by the GitHub API.
type InventoryRunner struct {
	ID             int64    `json:"id"`
	Name           string   `json:"name"`
	Scope          string   `json:"scope"` // registration scope: "repo" or "org"
	Repository     string   `json:"repository,omitempty"`   // owner/repo (repo-scoped runners)
	Organization   string   `json:"organization,omitempty"` // org name (org-scoped runners)
	Status         string   `json:"status"`                 // online | offline (passthrough)
	Busy           bool     `json:"busy"`
	Labels         []string `json:"labels,omitempty"`
	Version        string   `json:"version,omitempty"`
	OS             string   `json:"os,omitempty"`
	Ephemeral      bool     `json:"ephemeral,omitempty"`
}

// InventoryFailure is one source that could not be queried. Failures
// are reported, never fatal: other sources are still aggregated.
type InventoryFailure struct {
	Scope  string `json:"scope"`  // repo | org
	Target string `json:"target"` // owner/repo or org name
	Error  string `json:"error"`
}

// inventoryResult is the outcome of an inventory query.
type inventoryResult struct {
	Scope    string
	Target   string
	Runners  []InventoryRunner
	Failures []InventoryFailure
}

// ghRunner mirrors the subset of the actions-runner JSON we report.
type ghRunner struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	OS        string `json:"os"`
	Status    string `json:"status"`
	Busy      bool   `json:"busy"`
	Version   string `json:"version"`
	Ephemeral bool   `json:"ephemeral"`
	Labels    []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

func (r ghRunner) toInventory(scope, repository, org string) InventoryRunner {
	out := InventoryRunner{
		ID:           r.ID,
		Name:         r.Name,
		Scope:        scope,
		Repository:   repository,
		Organization: org,
		Status:       r.Status,
		Busy:         r.Busy,
		Version:      r.Version,
		OS:           r.OS,
		Ephemeral:    r.Ephemeral,
	}
	for _, l := range r.Labels {
		if l.Name != "" {
			out.Labels = append(out.Labels, l.Name)
		}
	}
	return out
}

// ghError renders a gh failure with the actionable part of gh's output.
func ghError(target string, err error, out string) error {
	msg := strings.TrimSpace(out)
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		msg = strings.TrimSpace(msg[:i])
	}
	lower := strings.ToLower(msg)
	var hint string
	switch {
	case strings.Contains(lower, "rate limit"):
		hint = " (GitHub API rate limit hit — check `gh auth status`)"
	case strings.Contains(lower, "403") || strings.Contains(lower, "resource not accessible") || strings.Contains(lower, "must have admin"):
		hint = " (insufficient permissions — the token may lack the required scope for this target)"
	case strings.Contains(lower, "401") || strings.Contains(lower, "bad credentials") || strings.Contains(lower, "authentication"):
		hint = " (authentication failed — run `gh auth login` or set GH_TOKEN/GITHUB_TOKEN)"
	case strings.Contains(lower, "404") || strings.Contains(lower, "not found"):
		hint = " (not found or no access — a 404 for a private target usually means the token cannot see it)"
	}
	if msg == "" {
		return fmt.Errorf("gh api %s: %w%s", target, err, hint)
	}
	// Cap the echoed gh output so a verbose page doesn't flood the terminal.
	if len(msg) > 300 {
		msg = msg[:300] + "…"
	}
	return fmt.Errorf("gh api %s: %w: %s%s", target, err, msg, hint)
}

// ghAPI runs `gh api` with args and returns the combined output.
func ghAPI(args ...string) (string, error) {
	return commandExecution.Output("gh", args, "")
}

// ghRunners queries a runners endpoint. With --paginate, gh applies
// --jq to each page and prints the results; `.runners[]` therefore
// yields one JSON object per runner across all pages.
func ghRunners(endpoint string) ([]ghRunner, error) {
	out, err := ghAPI("api", "--paginate", endpoint, "--jq", ".runners[]")
	if err != nil {
		return nil, ghError(endpoint, err, out)
	}
	return parseRunnerNDJSON(out, endpoint)
}

// parseRunnerNDJSON decodes newline-delimited runner objects. gh emits
// NDJSON when --paginate processes multiple pages, but the same parser
// also tolerates a pretty-printed JSON array from a single page.
func parseRunnerNDJSON(out, endpoint string) ([]ghRunner, error) {
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return nil, nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var runners []ghRunner
		if err := json.Unmarshal([]byte(trimmed), &runners); err != nil {
			return nil, fmt.Errorf("parsing runners from %s: %w", endpoint, err)
		}
		return runners, nil
	}
	var runners []ghRunner
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var r ghRunner
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			return nil, fmt.Errorf("parsing runner JSON from %s: %w (line %.200q)", endpoint, err, line)
		}
		runners = append(runners, r)
	}
	return runners, nil
}

// ghRepoFullNames enumerates the full_name of every repository returned
// by a repos listing endpoint, following pagination.
func ghRepoFullNames(endpoint string) ([]string, error) {
	out, err := ghAPI("api", "--paginate", endpoint, "--jq", ".[].full_name")
	if err != nil {
		return nil, ghError(endpoint, err, out)
	}
	var names []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		line = strings.Trim(line, `"`)
		if line == "" {
			continue
		}
		names = append(names, line)
	}
	return names, nil
}

// repoInventory queries one repository's runners.
func repoInventory(ownerRepo string) ([]InventoryRunner, error) {
	rs, err := ghRunners(fmt.Sprintf("repos/%s/actions/runners?per_page=100", ownerRepo))
	if err != nil {
		return nil, err
	}
	out := make([]InventoryRunner, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.toInventory("repo", ownerRepo, ""))
	}
	return out, nil
}

// orgInventory queries one organization's runners.
func orgInventory(org string) ([]InventoryRunner, error) {
	rs, err := ghRunners(fmt.Sprintf("orgs/%s/actions/runners?per_page=100", org))
	if err != nil {
		return nil, err
	}
	out := make([]InventoryRunner, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.toInventory("org", "", org))
	}
	return out, nil
}

// isOrg checks whether owner is an organization the token can see.
// notFound reports the owner is (most likely) a user account.
func isOrg(owner string) (bool, error) {
	out, err := ghAPI("api", "orgs/"+owner, "--jq", ".login")
	if err == nil {
		return true, nil
	}
	lower := strings.ToLower(out)
	if strings.Contains(lower, "404") || strings.Contains(lower, "not found") {
		return false, nil
	}
	return false, ghError("orgs/"+owner, err, out)
}

// collectOwnerInventory aggregates runners across everything an owner
// (user or organization) has, to the extent the token can see.
func collectOwnerInventory(owner string) inventoryResult {
	res := inventoryResult{Scope: "owner", Target: owner}
	org, orgErr := isOrg(owner)
	if orgErr != nil {
		// Not fatal: fall through to the user path, which may still work.
		res.Failures = append(res.Failures, InventoryFailure{Scope: "org", Target: owner, Error: orgErr.Error()})
	}

	if org {
		// Organization: org-level runners first...
		runners, err := orgInventory(owner)
		if err != nil {
			res.Failures = append(res.Failures, InventoryFailure{Scope: "org", Target: owner, Error: err.Error()})
		} else {
			res.Runners = append(res.Runners, runners...)
		}
		// ...then every repository the org owns (or the token can see).
		repos, err := ghRepoFullNames(fmt.Sprintf("orgs/%s/repos?per_page=100&type=all", owner))
		if err != nil {
			res.Failures = append(res.Failures, InventoryFailure{Scope: "repo", Target: owner + "/*", Error: err.Error()})
			return finishInventory(res)
		}
		res.Runners = append(res.Runners, collectRepoRunners(repos, &res.Failures)...)
		return finishInventory(res)
	}

	// User (or an owner we could not classify): no account-level
	// endpoint exists, so enumerate owned repositories.
	repos, err := ghRepoFullNames(fmt.Sprintf("users/%s/repos?per_page=100&type=owner", owner))
	if err != nil {
		res.Failures = append(res.Failures, InventoryFailure{Scope: "repo", Target: owner + "/*", Error: err.Error()})
		return finishInventory(res)
	}
	res.Runners = append(res.Runners, collectRepoRunners(repos, &res.Failures)...)
	return finishInventory(res)
}

// collectRepoRunners queries each repo, recording per-repo failures and
// continuing with the rest.
func collectRepoRunners(repos []string, failures *[]InventoryFailure) []InventoryRunner {
	var out []InventoryRunner
	for _, repo := range repos {
		full := strings.TrimSpace(repo)
		if full == "" {
			continue
		}
		runners, err := repoInventory(full)
		if err != nil {
			*failures = append(*failures, InventoryFailure{Scope: "repo", Target: full, Error: err.Error()})
			continue
		}
		out = append(out, runners...)
	}
	return out
}

// finishInventory dedupes identical runners reported through multiple
// paths and sorts the result for stable output.
func finishInventory(res inventoryResult) inventoryResult {
	seen := map[string]bool{}
	deduped := res.Runners[:0]
	for _, r := range res.Runners {
		key := fmt.Sprintf("%s|%s|%s|%d|%s", r.Scope, r.Repository, r.Organization, r.ID, r.Name)
		if seen[key] {
			continue
		}
		seen[key] = true
		deduped = append(deduped, r)
	}
	res.Runners = deduped
	sortRunnerInventory(res.Runners)
	if res.Failures == nil {
		res.Failures = []InventoryFailure{}
	}
	return res
}

func sortRunnerInventory(rs []InventoryRunner) {
	scope := func(r InventoryRunner) string {
		if r.Scope == "org" {
			return "org:" + r.Organization
		}
		return "repo:" + r.Repository
	}
	sort.Slice(rs, func(i, j int) bool {
		a, b := rs[i], rs[j]
		if scope(a) != scope(b) {
			return scope(a) < scope(b)
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.ID < b.ID
	})
}

// collectInventory dispatches to the right aggregator for scope. Target
// is "owner/repo" for scope=repo, the org name for scope=org and the
// account for scope=owner.
func collectInventory(scope, target string) inventoryResult {
	switch scope {
	case "repo":
		res := inventoryResult{Scope: "repo", Target: target}
		runners, err := repoInventory(target)
		if err != nil {
			res.Failures = append(res.Failures, InventoryFailure{Scope: "repo", Target: target, Error: err.Error()})
			return finishInventory(res)
		}
		res.Runners = runners
		return finishInventory(res)
	case "org":
		res := inventoryResult{Scope: "org", Target: target}
		runners, err := orgInventory(target)
		if err != nil {
			res.Failures = append(res.Failures, InventoryFailure{Scope: "org", Target: target, Error: err.Error()})
			return finishInventory(res)
		}
		res.Runners = runners
		return finishInventory(res)
	case "owner":
		return collectOwnerInventory(target)
	default:
		return inventoryResult{Scope: scope, Target: target, Failures: []InventoryFailure{{Scope: scope, Target: target, Error: "unknown inventory scope"}}}
	}
}

// writeInventory prints the inventory as text or JSON.
func writeInventory(w io.Writer, format string, res inventoryResult) error {
	if format == "json" {
		runners := res.Runners
		if runners == nil {
			runners = []InventoryRunner{}
		}
		failures := res.Failures
		if failures == nil {
			failures = []InventoryFailure{}
		}
		out := map[string]any{
			"scope":    res.Scope,
			"target":   res.Target,
			"ok":       len(failures) == 0,
			"runners":  runners,
			"failures": failures,
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	for _, r := range res.Runners {
		busy := "idle"
		if r.Busy {
			busy = "busy"
		}
		targetKey, targetVal := "repo", r.Repository
		if r.Scope == "org" {
			targetKey, targetVal = "org", r.Organization
		}
		fmt.Fprintf(w, "%d  %s  scope=%s  %s=%s  status=%s  %s", r.ID, r.Name, r.Scope, targetKey, targetVal, r.Status, busy)
		if len(r.Labels) > 0 {
			fmt.Fprintf(w, "  labels=%s", strings.Join(r.Labels, ","))
		}
		if r.Version != "" {
			fmt.Fprintf(w, "  version=%s", r.Version)
		}
		if r.OS != "" {
			fmt.Fprintf(w, "  os=%s", r.OS)
		}
		if r.Ephemeral {
			fmt.Fprintf(w, "  ephemeral")
		}
		fmt.Fprintln(w)
	}
	return nil
}

// runInventory executes the read-only GitHub inventory path of `status`.
func runInventory(cfg *Config, scope, orgFlag string, stdout, stderr io.Writer) error {
	// Resolve inventory targets from flags + config (owner/repo in
	// cfg may already have been split by the caller when --repo was
	// given as owner/repo).
	var target string
	switch scope {
	case "org":
		org := strings.TrimSpace(orgFlag)
		if org == "" {
			org = strings.TrimSpace(cfg.Owner)
		}
		if org == "" {
			return usageErrf("--scope org requires --org (or an owner from config, env or git remote)")
		}
		target = org
	case "owner":
		owner := strings.TrimSpace(cfg.Owner)
		if owner == "" {
			return usageErrf("--scope owner requires --owner (or an owner from config, env or git remote)")
		}
		target = owner
	case "repo":
		if cfg.Owner == "" || cfg.Repo == "" {
			return usageErrf("--scope repo requires an owner and a repository: --owner/--repo (or --repo owner/repo)")
		}
		target = cfg.Owner + "/" + cfg.Repo
	default:
		return usageErrf("unsupported GitHub inventory scope %q", scope)
	}

	// gh must be present and working; it carries the auth for us.
	if out, err := ghAPI("--version"); err != nil {
		return fmt.Errorf("GitHub CLI `gh` is not available: %w — install https://cli.github.com and authenticate (`gh auth login` or set GH_TOKEN/GITHUB_TOKEN)", err)
	} else {
		_ = out
	}

	res := collectInventory(scope, target)

	for _, f := range res.Failures {
		fmt.Fprintf(stderr, "runner-spawner: warning: %s %s: %s\n", f.Scope, f.Target, f.Error)
	}
	if err := writeInventory(stdout, cfg.Format, res); err != nil {
		return err
	}
	if len(res.Failures) > 0 {
		return fmt.Errorf("inventory incomplete: %d source(s) failed (see warnings above); reported %d runner(s)", len(res.Failures), len(res.Runners))
	}
	return nil
}
