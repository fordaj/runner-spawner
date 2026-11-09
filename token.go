package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// runnerToken resolves a registration token for owner/repo.
//
// Precedence: --token / cfg.Token, RUNNER_SPAWNER_TOKEN, GH_TOKEN or
// GITHUB_TOKEN, then `gh api repos/<owner>/<repo>/actions/runners/registration-token
// -X POST -q .token` (same call as the old _get_token.sh).
//
// The token is never logged. In dry-run mode callers must not call this.
func runnerToken(cfg *Config) (string, error) {
	if cfg.Token != "" {
		return cfg.Token, nil
	}
	for _, env := range []string{EnvToken, "GH_TOKEN", "GITHUB_TOKEN"} {
		if v := strings.TrimSpace(os.Getenv(env)); v != "" {
			return v, nil
		}
	}
	if _, err := exec.LookPath("gh"); err != nil {
		return "", fmt.Errorf("no token provided and `gh` was not found on PATH; pass --token or set %s / GH_TOKEN (see README)", EnvToken)
	}
	path := fmt.Sprintf("repos/%s/%s/actions/runners/registration-token", cfg.Owner, cfg.Repo)
	cmd := exec.CommandContext(context.Background(), "gh", "api", path, "-X", "POST", "-q", ".token")
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("registration token request failed for %s/%s: %s (run `gh auth login` or pass --token)", cfg.Owner, cfg.Repo, strings.TrimSpace(string(ee.Stderr)))
		}
		return "", fmt.Errorf("registration token request failed for %s/%s: %w", cfg.Owner, cfg.Repo, err)
	}
	tok := strings.TrimSpace(string(out))
	// gh -q .token prints the bare token; some versions print JSON.
	if strings.HasPrefix(tok, `"`) {
		var s string
		if err := json.Unmarshal([]byte(tok), &s); err == nil {
			tok = s
		}
	}
	if tok == "" {
		return "", fmt.Errorf("registration token is empty for %s/%s (check `gh auth status`)", cfg.Owner, cfg.Repo)
	}
	return tok, nil
}
