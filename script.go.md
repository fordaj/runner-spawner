# Subprocess seam (`script.go`)

```mermaid
flowchart TD
    Verbs("<img src='barnacle:file-code.svg' width='20' alt=''/><br/><u>verbs.go.md · token.go.md · runner.go.md</u>") --> Helpers("<img src='barnacle:file-code.svg' width='20' alt=''/><br/>runScriptInDir · runScriptCaptureInDir<br/>runCmd · runCmdCapture")
    Helpers --> Seam{{"<img src='barnacle:hexagon.svg' width='20' alt=''/><br/>commandExecution<br/>package var"}}
    Seam -->|production| Real("<img src='barnacle:file-code.svg' width='20' alt=''/><br/>realExecutor<br/>os/exec")
    Seam -->|tests| Fake("<img src='barnacle:file-code.svg' width='20' alt=''/><br/>fake_exec_test.go<br/>records invocations,<br/>scripts gh api + service state")
    Real --> Exit("<img src='barnacle:file-code.svg' width='20' alt=''/><br/>exitCoder<br/>*exec.ExitError")
    Fake --> Exit
    Seam -.-> Scripts("<img src='barnacle:terminal.svg' width='20' alt=''/><br/>config.sh / svc.sh / sc.exe<br/>gh / git / net")
    click Verbs href "verbs.go.md"
    class Verbs,Helpers,Real,Fake,Exit goFile
    class Seam hexNode
    class Scripts shFile
    classDef goFile fill:#8fd8ef,stroke:#0277a8,color:#08303d
    classDef hexNode fill:#ffe08a,stroke:#a16207,color:#422006
    classDef shFile fill:#abe07a,stroke:#3f6212,color:#16280a
```

`script.go` is the only place the tool spawns external commands (`.runner` scripts, `sc.exe`, `net`, `gh`, `git`). All of it goes through one interface so tests can run hermetically — the whole create/remove/start/stop state matrix is covered by `fake_exec_test.go` without network, real services, or a Windows machine.

## The seam

- `commandExecutor` — `Run(name, args, dir)` (streams stdio, for scripts the user should see) and `Output(name, args, dir)` (combined output, for probing). `commandExecution` is a package var pointing at `realExecutor{}`; tests swap it for a fake.
- `realExecutor` — plain `os/exec`; `Run` inherits stdin/stdout/stderr, `Output` uses `CombinedOutput()`.
- `exitCoder` — implemented by `*exec.ExitError` and by test fakes, so callers can assert on specific exit codes (e.g. `sc.exe`'s 1060 "service does not exist") without spawning processes.

## Call-site helpers

- `runScriptInDir(script, args, dir)` — run `config.sh`/`config.cmd`, `svc.sh`. Runner scripts must be invoked with `dir` == the runner root.
- `runScriptCaptureInDir(script, args, dir)` — same, with combined output (e.g. `svc.sh status`).

## Test posture

Because every subprocess funnels through `commandExecution`, `go test ./...` runs fully offline; the fake executor records invocations and simulates `.runner`/service state, including scripted `gh api` endpoints and per-runner-dir service failures. See [AGENTS.md — Build/Test](AGENTS.md).

## Related pages

- [runner.go.md](runner.go.md) — `runCmd`/`runCmdCapture` wrappers
- [verbs.go.md](verbs.go.md) — the lifecycle operations built on this seam
- [token.go.md](token.go.md) — `gh api` goes through the same seam
