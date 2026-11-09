package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// parseRemoteURL extracts (owner, repo) from a GitHub remote URL. It accepts
// the forms git produces: scp-style (git@github.com:owner/repo.git),
// ssh://, https://, and git:// — with or without .git and trailing slash.
func parseRemoteURL(raw string) (owner, repo string, err error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", "", fmt.Errorf("empty remote URL")
	}
	// Strip trailing slash, then .git suffix.
	s = strings.TrimSuffix(s, "/")
	s = strings.TrimSuffix(s, ".git")
	// Strip scheme if present.
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
		// ssh://git@github.com/... or https://github.com/... or git://...
		if at := strings.LastIndex(s, "@"); at >= 0 {
			s = s[at+1:]
		}
	} else if at := strings.Index(s, "@"); at >= 0 {
		// scp-style git@github.com:owner/repo
		if colon := strings.Index(s[at+1:], ":"); colon >= 0 {
			s = s[at+1+colon+1:]
		} else {
			return "", "", fmt.Errorf("unrecognised remote URL %q", raw)
		}
	}
	// Now s should be host/owner/repo.
	parts := strings.Split(s, "/")
	owner, repo = "", ""
	if len(parts) >= 2 {
		owner = parts[len(parts)-2]
		repo = parts[len(parts)-1]
	}
	if owner == "" || repo == "" {
		return "", "", fmt.Errorf("unrecognised remote URL %q (expected <host>/owner/repo)", raw)
	}
	if strings.ContainsAny(owner, " \t\n") || strings.ContainsAny(repo, " \t\n") {
		return "", "", fmt.Errorf("unrecognised remote URL %q (expected <host>/owner/repo)", raw)
	}
	return owner, repo, nil
}

// detectGitRemoteOwnerRepo runs `git -C <dir> remote get-url origin` and
// parses the result. It returns ok=false (and no error) when the directory
// is not a git clone or has no origin remote, so callers can fall back to
// config/env/flags.
func detectGitRemoteOwnerRepo(dir string) (owner, repo string, ok bool) {
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		return "", "", false
	}
	o, r, err := parseRemoteURL(string(out))
	if err != nil {
		return "", "", false
	}
	return o, r, true
}
