package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// unrecognisedRemote builds the error returned for an unparseable remote URL.
func unrecognisedRemote(raw string) error {
	return fmt.Errorf("unrecognised remote URL %q (expected <host>/owner/repo)", raw)
}

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
	switch {
	case strings.Contains(s, "://"):
		// https://, ssh://, git:// — drop the scheme and any userinfo.
		s = s[strings.Index(s, "://")+3:]
		if at := strings.LastIndex(s, "@"); at >= 0 {
			s = s[at+1:]
		}
	case strings.Contains(s, "@"):
		// scp-style git@host:owner/repo — the colon ends the host part.
		after := s[strings.Index(s, "@")+1:]
		colon := strings.Index(after, ":")
		if colon < 0 {
			return "", "", unrecognisedRemote(raw)
		}
		s = after[colon+1:]
	}
	// Now s should be host/owner/repo (or owner/repo for scp-style);
	// only the last two segments matter.
	parts := strings.Split(s, "/")
	if len(parts) < 2 {
		return "", "", unrecognisedRemote(raw)
	}
	owner, repo = parts[len(parts)-2], parts[len(parts)-1]
	if owner == "" || repo == "" {
		return "", "", unrecognisedRemote(raw)
	}
	if strings.ContainsAny(owner, " \t\n") || strings.ContainsAny(repo, " \t\n") {
		return "", "", unrecognisedRemote(raw)
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
