# `runner-spawner init` (`init.go`)

```mermaid
flowchart TD
    Init("<img src='barnacle:file-code.svg' width='20' alt=''/><br/>runner-spawner init") --> Flags{{"<img src='barnacle:hexagon.svg' width='20' alt=''/><br/>only --help / --dir?"}}
    Flags -->|no| Usage("<img src='barnacle:circle-exclamation-point.svg' width='20' alt=''/><br/>exit 2<br/>usage error")
    Flags -->|yes| Remote("<img src='barnacle:file-code.svg' width='20' alt=''/><br/><u>detectGitRemoteOwnerRepo<br/>git.go.md</u>")
    Remote --> Write("<img src='barnacle:braces.svg' width='20' alt=''/><br/><u>write .runner-spawner.json<br/>owner repo runnerNames …</u>")
    Write --> Exists{{"<img src='barnacle:hexagon.svg' width='20' alt=''/><br/>file exists?"}}
    Exists -->|no| Done("<img src='barnacle:check.svg' width='20' alt=''/><br/>written")
    Exists -->|yes, non-tty| Refuse("<img src='barnacle:circle-exclamation-point.svg' width='20' alt=''/><br/>exit 2<br/>never overwrite")
    Exists -->|yes, tty| Prompt{{"<img src='barnacle:hexagon.svg' width='20' alt=''/><br/>Overwrite? y/N"}}
    Prompt -->|y| Done
    Prompt -->|N| Refuse
    click Remote href "git.go.md"
    click Write href "config.go.md"
    class Init,Remote goFile
    class Flags,Exists,Prompt hexNode
    class Usage,Refuse errNode
    class Write jsonFile
    class Done checkNode
    classDef goFile fill:#8fd8ef,stroke:#0277a8,color:#08303d
    classDef hexNode fill:#ffe08a,stroke:#a16207,color:#422006
    classDef errNode fill:#f6b8b8,stroke:#b91c1c,color:#450a0a
    classDef jsonFile fill:#cdb9f7,stroke:#6d28d9,color:#2e1065
    classDef checkNode fill:#9fe8c9,stroke:#0f766e,color:#052e22
```

`init.go` implements `runner-spawner init [--dir DIR]`: write a `.runner-spawner.json` for a repository, detecting owner/repo from its git remote — the same bootstrap step the README walks through by hand.

## Behavior

- `runInit` parses only `--help`/`-h` and `--dir DIR` (default `.`); anything else is a usage error (exit `2`).
- `confirmOverwrite(stdin, target, stdout, stderr)` owns the overwrite guard: refuses with exit `2` on a non-terminal stdin, prompts `Overwrite? [y/N]` in a terminal.
- It examines the target directory's `origin` remote via `detectGitRemoteOwnerRepo` (see [git.go.md](git.go.md)) and writes `<dir>/.runner-spawner.json` (`initFile`: owner, repo, `runnerNames: ["runner01"]`, `runnersDir: .runners`, `runnerGroup: Default`, `runnerVersion: 2.338.2`).
- **Never overwrites non-interactively**: if the file exists and stdin is not a terminal, it exits with code `2`; in a terminal it prompts `Overwrite? [y/N]` and proceeds only on an explicit yes.
- The file contains no secrets; the committed `.runner-spawner.json` at the repo root is this tool's own dogfooding.

`stdinIsTerminal` isolates the TTY check from `stdin`.

## Related pages

- [config.go.md](config.go.md) — the file it writes and how it is loaded
- [git.go.md](git.go.md) — owner/repo detection
- [main.go.md](main.go.md) — how `init` is wired into subcommand dispatch
