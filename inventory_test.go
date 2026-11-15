package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// inventoryTestCfg returns a Config for inventory output tests.
func inventoryTestCfg(format string) *Config {
	cfg := DefaultConfig()
	cfg.Quiet = true
	cfg.Format = format
	return cfg
}

func ghRunnerLine(id int64, name, status string, busy bool, labels []string, version, osName string) map[string]any {
	labelObjects := []map[string]any{}
	for _, l := range labels {
		labelObjects = append(labelObjects, map[string]any{"name": l})
	}
	return map[string]any{
		"id":      id,
		"name":    name,
		"os":      osName,
		"status":  status,
		"busy":    busy,
		"version": version,
		"labels":  labelObjects,
	}
}

func ndjson(objs ...map[string]any) string {
	var sb strings.Builder
	for _, o := range objs {
		data, err := json.Marshal(o)
		if err != nil {
			panic(err)
		}
		sb.Write(data)
		sb.WriteByte('\n')
	}
	return sb.String()
}

func swapCommandExecution(t *testing.T, exec commandExecutor) {
	t.Helper()
	prev := commandExecution
	commandExecution = exec
	t.Cleanup(func() { commandExecution = prev })
}

func TestInventoryRepoScope(t *testing.T) {
	fake := newFakeExecutor()
	fake.GitHubResponses = map[string]fakeGitHubResponse{
		"repos/acme/widgets/actions/runners?per_page=100": {
			Out: ndjson(
				ghRunnerLine(11, "runner-a", "online", true, []string{"self-hosted", "linux"}, "2.338.2", "linux"),
				ghRunnerLine(12, "runner-b", "offline", false, []string{"self-hosted", "windows"}, "2.338.2", "windows"),
			),
		},
	}
	fake.swap(t)

	cfg := inventoryTestCfg("json")
	var stdout, stderr bytes.Buffer
	cfg.Owner, cfg.Repo = "acme", "widgets"
	if err := runInventory(cfg, "repo", "", &stdout, &stderr); err != nil {
		t.Fatalf("runInventory: %v (stderr: %s)", err, stderr.String())
	}
	var decoded struct {
		Scope    string            `json:"scope"`
		Target   string            `json:"target"`
		OK       bool              `json:"ok"`
		Runners  []InventoryRunner `json:"runners"`
		Failures []InventoryFailure `json:"failures"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
	}
	if decoded.Scope != "repo" || decoded.Target != "acme/widgets" || !decoded.OK {
		t.Errorf("unexpected envelope: %+v", decoded)
	}
	if len(decoded.Runners) != 2 {
		t.Fatalf("got %d runners, want 2", len(decoded.Runners))
	}
	a := decoded.Runners[0]
	if a.ID != 11 || a.Name != "runner-a" || a.Scope != "repo" || a.Repository != "acme/widgets" {
		t.Errorf("runner 0 = %+v", a)
	}
	if !a.Busy || a.Status != "online" || a.Version != "2.338.2" || a.OS != "linux" {
		t.Errorf("runner 0 fields = %+v", a)
	}
	if len(a.Labels) != 2 || a.Labels[0] != "self-hosted" {
		t.Errorf("runner 0 labels = %v", a.Labels)
	}
	if len(decoded.Failures) != 0 {
		t.Errorf("failures = %v, want none", decoded.Failures)
	}
}

func TestInventoryRepoScopeTextOutput(t *testing.T) {
	fake := newFakeExecutor()
	fake.GitHubResponses = map[string]fakeGitHubResponse{
		"repos/acme/widgets/actions/runners?per_page=100": {
			Out: ndjson(ghRunnerLine(11, "runner-a", "online", false, []string{"self-hosted"}, "2.338.2", "linux")),
		},
	}
	fake.swap(t)

	cfg := inventoryTestCfg("text")
	var stdout, stderr bytes.Buffer
	cfg.Owner, cfg.Repo = "acme", "widgets"
	if err := runInventory(cfg, "repo", "", &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	out := stdout.String()
	for _, want := range []string{"11", "runner-a", "scope=repo", "repo=acme/widgets", "status=online", "idle", "labels=self-hosted", "version=2.338.2"} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q missing %q", out, want)
		}
	}
}

func TestInventoryOrgScope(t *testing.T) {
	fake := newFakeExecutor()
	fake.GitHubResponses = map[string]fakeGitHubResponse{
		"orgs/acme/actions/runners?per_page=100": {
			Out: ndjson(ghRunnerLine(21, "org-runner", "online", true, []string{"self-hosted"}, "2.338.2", "linux")),
		},
	}
	fake.swap(t)

	cfg := inventoryTestCfg("json")
	var stdout, stderr bytes.Buffer
	cfg.Owner, cfg.Repo = "acme", "widgets"
	if err := runInventory(cfg, "org", "acme", &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Runners  []InventoryRunner `json:"runners"`
		Failures []InventoryFailure `json:"failures"`
		OK       bool              `json:"ok"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Runners) != 1 || decoded.Runners[0].Scope != "org" || decoded.Runners[0].Organization != "acme" {
		t.Fatalf("runners = %+v, want one org-scoped runner", decoded.Runners)
	}
	if decoded.Runners[0].Repository != "" {
		t.Errorf("org runner has repository %q, want empty", decoded.Runners[0].Repository)
	}
}

func TestInventoryOrgScopeReportsFailure(t *testing.T) {
	fake := newFakeExecutor()
	fake.GitHubResponses = map[string]fakeGitHubResponse{
		"orgs/acme/actions/runners?per_page=100": {
			Out: `{"message":"Not Found","status":"404"}`,
			Err: errors.New("exit status 1"),
		},
	}
	fake.swap(t)

	cfg := inventoryTestCfg("json")
	var stdout, stderr bytes.Buffer
	err := runInventory(cfg, "org", "acme", &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for inventory with failures")
	}
	if !strings.Contains(err.Error(), "1 source(s) failed") {
		t.Errorf("error = %v, want inventory incomplete summary", err)
	}
	var decoded struct {
		Failures []InventoryFailure `json:"failures"`
		OK       bool              `json:"ok"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Failures) != 1 || decoded.Failures[0].Target != "acme" {
		t.Fatalf("failures = %+v, want one failure for acme", decoded.Failures)
	}
	if decoded.OK {
		t.Error("ok = true, want false with failures")
	}
	if !strings.Contains(stderr.String(), "warning:") {
		t.Errorf("stderr = %q, want a warning naming the failure", stderr.String())
	}
}

func TestInventoryOwnerAggregatesUser(t *testing.T) {
	fake := newFakeExecutor()
	fake.GitHubResponses = map[string]fakeGitHubResponse{
		// Not an organization: gh reports 404, user path is used.
		"orgs/acme": {Out: `{"message":"Not Found","status":"404"}`, Err: errors.New("exit status 1")},
		"users/acme/repos?per_page=100&type=owner": {
			Out: "\"acme/one\"\n\"acme/two\"\n",
		},
		"repos/acme/one/actions/runners?per_page=100": {
			Out: ndjson(
				ghRunnerLine(31, "one-runner", "online", false, nil, "2.338.2", "linux"),
				ghRunnerLine(31, "one-runner", "online", false, nil, "2.338.2", "linux"), // duplicate: deduped
			),
		},
		// This repo is not accessible to the token: partial failure.
		"repos/acme/two/actions/runners?per_page=100": {
			Out: `{"message":"Resource not accessible by integration","status":"403"}`,
			Err: errors.New("exit status 1"),
		},
	}
	fake.swap(t)

	cfg := inventoryTestCfg("json")
	cfg.Owner = "acme"
	var stdout, stderr bytes.Buffer
	err := runInventory(cfg, "owner", "", &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error: one repo failed")
	}
	var decoded struct {
		Runners  []InventoryRunner `json:"runners"`
		Failures []InventoryFailure `json:"failures"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Runners) != 1 || decoded.Runners[0].Name != "one-runner" {
		t.Fatalf("runners = %+v, want the single deduped one-runner", decoded.Runners)
	}
	if len(decoded.Failures) != 1 || decoded.Failures[0].Target != "acme/two" {
		t.Fatalf("failures = %+v, want one failure for acme/two", decoded.Failures)
	}
	if !strings.Contains(decoded.Failures[0].Error, "403") {
		t.Errorf("failure = %q, want the 403 permission detail", decoded.Failures[0].Error)
	}
	if !strings.Contains(stderr.String(), "warning:") {
		t.Errorf("stderr = %q, want a partial-failure warning", stderr.String())
	}
}

func TestInventoryOwnerAggregatesOrg(t *testing.T) {
	fake := newFakeExecutor()
	fake.GitHubResponses = map[string]fakeGitHubResponse{
		"orgs/acme": {Out: "acme\n"},
		"orgs/acme/actions/runners?per_page=100": {
			Out: ndjson(ghRunnerLine(41, "org-runner", "online", false, []string{"self-hosted"}, "2.338.2", "linux")),
		},
		"orgs/acme/repos?per_page=100&type=all": {
			Out: "\"acme/one\"\n\"acme/two\"\n",
		},
		"repos/acme/one/actions/runners?per_page=100": {
			Out: ndjson(ghRunnerLine(42, "repo-runner", "offline", false, nil, "2.338.2", "linux")),
		},
		"repos/acme/two/actions/runners?per_page=100": {
			Out: "", // accessible, zero runners — not a failure
		},
	}
	fake.swap(t)

	cfg := inventoryTestCfg("json")
	cfg.Owner = "acme"
	var stdout, stderr bytes.Buffer
	if err := runInventory(cfg, "owner", "", &stdout, &stderr); err != nil {
		t.Fatalf("runInventory: %v", err)
	}
	var decoded struct {
		Runners  []InventoryRunner `json:"runners"`
		Failures []InventoryFailure `json:"failures"`
		OK       bool              `json:"ok"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if !decoded.OK {
		t.Fatalf("failures = %+v, want none", decoded.Failures)
	}
	if len(decoded.Runners) != 2 {
		t.Fatalf("runners = %+v, want org-runner + repo-runner", decoded.Runners)
	}
	var orgRunner, repoRunner *InventoryRunner
	for i := range decoded.Runners {
		switch decoded.Runners[i].Scope {
		case "org":
			orgRunner = &decoded.Runners[i]
		case "repo":
			repoRunner = &decoded.Runners[i]
		}
	}
	if orgRunner == nil || repoRunner == nil {
		t.Fatalf("runners = %+v, want one org-scoped and one repo-scoped", decoded.Runners)
	}
	if orgRunner.Organization != "acme" || repoRunner.Repository != "acme/one" {
		t.Errorf("scope targets wrong: org=%q repo=%q", orgRunner.Organization, repoRunner.Repository)
	}
}

func TestInventoryOwnerRepoEnumerationFails(t *testing.T) {
	fake := newFakeExecutor()
	fake.GitHubResponses = map[string]fakeGitHubResponse{
		"orgs/acme": {Out: `{"message":"Not Found","status":"404"}`, Err: errors.New("exit status 1")},
		"users/acme/repos?per_page=100&type=owner": {
			Out: `{"message":"Bad credentials","status":"401"}`,
			Err: errors.New("exit status 1"),
		},
	}
	fake.swap(t)

	cfg := inventoryTestCfg("json")
	cfg.Owner = "acme"
	var stdout, stderr bytes.Buffer
	err := runInventory(cfg, "owner", "", &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error when repository enumeration fails")
	}
	var decoded struct {
		Failures []InventoryFailure `json:"failures"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Failures) != 1 {
		t.Fatalf("failures = %+v, want enumeration failure", decoded.Failures)
	}
}

func TestInventoryEmptyRepoIsNotFailure(t *testing.T) {
	fake := newFakeExecutor()
	fake.GitHubResponses = map[string]fakeGitHubResponse{
		"repos/acme/empty/actions/runners?per_page=100": {Out: ""},
	}
	fake.swap(t)

	cfg := inventoryTestCfg("json")
	cfg.Owner, cfg.Repo = "acme", "empty"
	var stdout, stderr bytes.Buffer
	if err := runInventory(cfg, "repo", "", &stdout, &stderr); err != nil {
		t.Fatalf("empty inventory must not fail: %v", err)
	}
	var decoded struct {
		OK      bool              `json:"ok"`
		Runners []InventoryRunner `json:"runners"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if !decoded.OK || len(decoded.Runners) != 0 {
		t.Fatalf("got ok=%v runners=%v, want empty success", decoded.OK, decoded.Runners)
	}
}

func TestInventoryRepositoryNotFoundIsFailure(t *testing.T) {
	fake := newFakeExecutor()
	fake.GitHubResponses = map[string]fakeGitHubResponse{
		"repos/acme/missing/actions/runners?per_page=100": {
			Out: `{"message":"Not Found","status":"404"}`,
			Err: errors.New("exit status 1"),
		},
	}
	fake.swap(t)

	cfg := inventoryTestCfg("json")
	cfg.Owner, cfg.Repo = "acme", "missing"
	var stdout, stderr bytes.Buffer
	err := runInventory(cfg, "repo", "", &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for inaccessible repository")
	}
	if !strings.Contains(stderr.String(), "404") {
		t.Errorf("stderr = %q, want the 404 detail", stderr.String())
	}
}

func TestParseRunnerNDJSONHandlesMultiplePages(t *testing.T) {
	// Simulates gh --paginate output: NDJSON from two pages concatenated.
	page1 := ndjson(ghRunnerLine(1, "page1-runner", "online", false, nil, "2.338.2", "linux"))
	page2 := ndjson(ghRunnerLine(2, "page2-runner", "offline", true, nil, "2.338.2", "linux"))
	out := page1 + page2
	runners, err := parseRunnerNDJSON(out, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(runners) != 2 || runners[0].Name != "page1-runner" || runners[1].Name != "page2-runner" {
		t.Fatalf("runners = %+v, want one from each page", runners)
	}

	// A single pretty-printed array is tolerated too.
	pretty := `[{"id":1,"name":"array-runner","status":"online"}]`
	runners, err = parseRunnerNDJSON(pretty, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(runners) != 1 || runners[0].Name != "array-runner" {
		t.Fatalf("runners = %+v, want array-runner", runners)
	}

	// Garbage is an error, not silent data loss.
	if _, err := parseRunnerNDJSON("{not json}\n", "test"); err == nil {
		t.Fatal("expected parse error for garbage output")
	}
	if runners, err := parseRunnerNDJSON("", "test"); err != nil || runners != nil {
		t.Fatalf("empty output = %v, %v; want nil, nil", runners, err)
	}
}

func TestGHVersionProbeFailureIsClearError(t *testing.T) {
	stub := stubExecutor{err: errors.New("exec: gh: executable file not found in $PATH")}
	swapCommandExecution(t, stub)
	cfg := inventoryTestCfg("text")
	cfg.Owner, cfg.Repo = "acme", "widgets"
	var stdout, stderr bytes.Buffer
	err := runInventory(cfg, "repo", "", &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error when gh is unavailable")
	}
	if !strings.Contains(err.Error(), "gh") || !strings.Contains(err.Error(), "github.com/cli") && !strings.Contains(err.Error(), "cli.github.com") {
		t.Errorf("error = %q, want guidance to install gh", err.Error())
	}
}

func TestGHRepoFullNamesParsesQuotedNames(t *testing.T) {
	fake := newFakeExecutor()
	fake.GitHubResponses = map[string]fakeGitHubResponse{
		"users/acme/repos?per_page=100&type=owner": {Out: "\"acme/x\"\n\"acme/y\"\n"},
	}
	fake.swap(t)
	names, err := ghRepoFullNames("users/acme/repos?per_page=100&type=owner")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "acme/x" || names[1] != "acme/y" {
		t.Fatalf("names = %v, want [acme/x acme/y]", names)
	}
}

// stubExecutor fails every invocation; used to simulate a missing gh binary.
type stubExecutor struct{ err error }

func (s stubExecutor) Run(name string, args []string, dir string) error { return s.err }
func (s stubExecutor) Output(name string, args []string, dir string) (string, error) {
	return "", s.err
}
