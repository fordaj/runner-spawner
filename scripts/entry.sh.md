# The script entry point (`scripts/entry.sh`)

```mermaid
flowchart TD
    Entry[scripts/entry.sh] --> Env[env.sh<br/>cascade + absolute *_BIN]
    Entry --> Log[log LEVEL message<br/>timestamped records]
    Log --> Bin{logamuffin binary<br/>available?}
    Bin -->|yes| Muff[pinned logamuffin<br/>LOGAMUFFIN_BIN]
    Bin -->|no| Printf[printf fallback<br/>same call shape]
    Scripts[version.sh · docs.sh · format.sh<br/>tools update scripts] --> Entry
    click Env href "env.sh.md"
    click Scripts href "README.md"
```

Every repository script sources `scripts/entry.sh` instead of the individual cascade files, because it does two jobs at once:

1. **It loads the environment** — it sources [env.sh.md](env.sh.md), which sources the hidden cascade (`.common.env` plus the platform file).
2. **It defines `log()`** — the shared logging function used by the task scripts and by the `tools/` update scripts.

## `log()` semantics

`log LEVEL MESSAGE...` forwards to the pinned `logamuffin` (`tools/logamuffin/update.sh.md`) binary referenced by `LOGAMUFFIN_BIN`. The caller context is passed through in `LOG_SOURCE` (`file[:function]:line`), and logamuffin's own exit code (`0`, or `2` for usage/config errors) is preserved even under `set -e`.

When the binary is missing (a fresh clone before tools are populated), `log()` falls back to a `printf` implementation with the same call shape: word-to-severity mapping (TRACE…ALWAYS → numeric thresholds), `LOG_LEVEL`/`LOGAMUFFIN_LEVEL` filtering, and stream routing (ERROR and above to stderr). The fallback exists so the scripts work on a brand-new checkout without network access. `log()` with no arguments prints a usage line to stderr and returns `2`.

## Usage

```bash
#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
. "$SCRIPT_DIR/env.sh"     # or entry.sh when you need log()
```

## Related pages

- [env.sh.md](env.sh.md) — the cascade `entry.sh` loads
- [version.sh.md](version.sh.md) — the version script that sources `entry.sh`
- [docs.sh.md](docs.sh.md) — the docs generator that sources `entry.sh`
- [release.sh.md](release.sh.md) — the release pipeline (uses its own `printf` logging; does not source `entry.sh`)
