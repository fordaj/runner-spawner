# Git remote detection (`git.go`)

```mermaid
flowchart TD
    Start([owner/repo needed?]) --> Check{flags / env / config<br/>already set?}
    Check -->|yes| Done([keep them -<br/>git fills nothing])
    Check -->|no| Remote[git -C dir remote get-url origin]
    Remote --> Parse[parseRemoteURL<br/>scp-style · ssh:// · https:// · git://]
    Parse --> OK{parsed?}
    OK -->|yes| Fill([owner + repo filled<br/>lowest precedence])
    OK -->|no| Fallback([ok = false<br/>caller falls back / errors])
    click Fill href "config.go.md"
    click Done href "main.go.md"
```

`git.go` resolves the GitHub owner/repo from the target clone's `origin` remote — the lowest-precedence configuration source (see [config.go.md](config.go.md)).

## Functions

- `parseRemoteURL(raw)` extracts `(owner, repo)` from the forms git produces: scp-style (`git@github.com:owner/repo.git`), `ssh://`, `https://`, `git://` — with or without a trailing `.git` or trailing slash. It rejects empty input, whitespace, and anything that does not yield `<host>/owner/repo`.
- `detectGitRemoteOwnerRepo(dir)` runs `git -C <dir> remote get-url origin` and parses the result. It returns `ok=false` (and no error) when the directory is not a git clone or has no `origin`, so callers fall back to config/env/flags instead of failing.

## Where it is used

Called by the flag/config layer in [main.go.md](main.go.md) only while owner/repo are still unknown; fills just the missing half if one is set.

## Related pages

- [config.go.md](config.go.md) — the precedence this detection sits at the bottom of
- [main.go.md](main.go.md) — where the detection is invoked
