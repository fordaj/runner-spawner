# Git remote detection (`git.go`)

```mermaid
flowchart TD
    Start{{"<img src='barnacle:hexagon.svg' width='20' alt=''/><br/>owner/repo needed?"}} --> Check{{"<img src='barnacle:hexagon.svg' width='20' alt=''/><br/>flags / env / config<br/>already set?"}}
    Check -->|yes| Done("<img src='barnacle:check.svg' width='20' alt=''/><br/><u>keep them -<br/>git fills nothing</u>")
    Check -->|no| Remote("<img src='barnacle:terminal.svg' width='20' alt=''/><br/>git -C dir remote get-url origin")
    Remote --> Parse("<img src='barnacle:file-code.svg' width='20' alt=''/><br/>parseRemoteURL<br/>scp-style · ssh:// · https:// · git://")
    Parse --> OK{{"<img src='barnacle:hexagon.svg' width='20' alt=''/><br/>parsed?"}}
    OK -->|yes| Fill("<img src='barnacle:check.svg' width='20' alt=''/><br/><u>owner + repo filled<br/>lowest precedence</u>")
    OK -->|no| Fallback("<img src='barnacle:circle-exclamation-point.svg' width='20' alt=''/><br/>ok = false<br/>caller falls back / errors")
    click Fill href "config.go.md"
    click Done href "main.go.md"
    class Start,Check,OK hexNode
    class Done,Fill checkNode
    class Remote shFile
    class Parse goFile
    class Fallback errNode
    classDef hexNode fill:#ffe08a,stroke:#a16207,color:#422006
    classDef checkNode fill:#9fe8c9,stroke:#0f766e,color:#052e22
    classDef shFile fill:#abe07a,stroke:#3f6212,color:#16280a
    classDef goFile fill:#8fd8ef,stroke:#0277a8,color:#08303d
    classDef errNode fill:#f6b8b8,stroke:#b91c1c,color:#450a0a
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
