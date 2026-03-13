# yelo — Architecture

## What yelo is

yelo (Tagalog for "ice") is an FTP-style CLI for Amazon S3 and Glacier. You `cd` into buckets, `ls` prefixes, `get` and `put` objects, `freeze` and `thaw` archives. It remembers where you are between sessions. An interactive TUI lets you browse, manage profiles, track restores, and manage cached files.

## Architectural principles

### Choreography over orchestration

No central controller. Components coordinate through shared files and conventions, not through a conductor that calls them in sequence. Each piece reads what it needs, does its job, writes its result. The filesystem is the message bus.

This means:

- **Config is a file** (`~/.config/yelo/config.yaml`). Any tool can read or write it — yelo, a script, a text editor, another CLI. No API, no lock.
- **State is a file** (`~/.config/yelo/state.json`). The current bucket and prefix persist between sessions as a JSON blob. `cd` writes it; `ls`, `get`, `pwd` read it. They don't talk to each other — they talk to the file.
- **Commands are independent**. Each command resolves its own context (bucket, prefix, region, profile) by reading config and state. There's no shared in-memory state, no command bus, no event system.

### Files as the coordination layer

| File | Purpose | Format |
|---|---|---|
| `~/.config/yelo/config.yaml` | Buckets, defaults, daemon settings | YAML |
| `~/.config/yelo/state.json` | Current bucket and prefix | JSON |
| `~/.yelo/cache/{bucket}/{key}` | Downloaded file cache | Raw files |
| `~/.yelo/notifications/{id}.json` | Restore request tracking | JSON |
| `~/.yelo/daemon.pid` | Daemon process ID | Text |
| `~/.yelo/daemon.log` | Daemon activity log | Text |

Why files:
- **Any tool can participate.** A shell script can set the bucket by writing `state.json`.
- **Human-readable.** `cat`, `vim`, `jq`. No query language, no API.
- **Debuggable.** When something is wrong, you read the file.
- **Crash-proof.** If yelo dies mid-command, the last-written state is still there.

### Resolution chains, not configuration hierarchies

When a command needs a value (bucket, region, profile), it walks a resolution chain — a priority-ordered list of places to look. The first source that has an answer wins.

```
Bucket:  --bucket flag → state file → config default → sole configured bucket
Region:  --region flag → per-bucket config → global config → AWS SDK default
Profile: --profile flag → per-bucket config → global config → AWS SDK default
```

### Pipe-friendly by default

| Context | Behavior |
|---|---|
| Interactive terminal | Human-readable: aligned columns, labels |
| Piped / redirected | Machine-readable: bare keys, tab-separated values |

All diagnostic output goes to stderr. Data goes to stdout.

## Code layout

```
src/
  main.rs           Entry point, TUI event loop, key handlers
  cli.rs            Clap definitions, CLI command dispatch
  app.rs            TUI state (App struct, modes, background tasks)
  ui.rs             TUI rendering (ratatui)
  aws_ops.rs        S3/Glacier SDK calls (blocking tokio runtime)
  config.rs         YAML config
  state.rs          JSON state
  helpers.rs        Pure helpers (is_glacier, resolve_prefix, format_size)
  output.rs         TTY detection + CLI output formatting
  cache.rs          Local file cache
  restore.rs        Restore notification management
  credentials.rs    AWS credentials INI management
  daemon.rs         Background auto-download daemon
  log.rs            File logging
```

### Module boundaries

Each module takes data in and pushes data out. No circular dependencies.

- **cli.rs** — CLI commands. Reads config/state, calls aws_ops, writes state, formats output.
- **app.rs** — TUI state machine. Modes, background task channels, thread spawning.
- **ui.rs** — Pure rendering. Reads App state, draws frames. No side effects.
- **aws_ops.rs** — SDK calls. Knows nothing about config files or UI.
- **config.rs / state.rs** — File I/O. Know nothing about AWS.
- **helpers.rs** — Pure functions shared by CLI and TUI.
- **daemon.rs** — Standalone background process. Polls restores, auto-downloads.

## Commands

```
yelo pwd                     Show current bucket and prefix
yelo cd <path>               Change working directory (FTP-style)
yelo ls [path]               List objects and prefixes
yelo stat <key>              Show object metadata
yelo get <key> [dest]        Download object (- for stdout)
yelo put <file> [key]        Upload object (default: DEEP_ARCHIVE)
yelo freeze <file> [key]     Archive to Glacier Deep Archive
yelo thaw <key>              Restore from Glacier
yelo restore <key>           (same as thaw)
yelo buckets                 Manage configured buckets
yelo daemon start|stop|status
yelo tui                     Interactive terminal UI
```

## TUI architecture

Four tabs: Browse, Profiles, Restores, Library.

**Event loop** (`main.rs`): Terminal events are read in a background thread to prevent blocking the spinner animation. Events arrive via `mpsc::channel` with `recv_timeout(100ms)`.

**State** (`app.rs`): The `App` struct holds all TUI state. `Mode` enum variants represent modal overlays (confirm dialogs, forms, pickers). Background tasks spawn threads that send `BgResult` variants back through a channel. `poll_bg()` processes results each frame.

**Rendering** (`ui.rs`): Pure functions that take `&App` and draw to a `Frame`. No mutations.

## Design decisions

### Why FTP-style navigation

S3 is a flat key-value store, but humans think in directories. FTP-style `cd`/`ls`/`pwd` maps the mental model people already have onto S3's prefix-based listing.

### Why Glacier-first

yelo defaults `put` to DEEP_ARCHIVE because the primary use case is cold archival. Most S3 CLIs default to STANDARD and treat Glacier as an afterthought. yelo treats Glacier as the default and STANDARD as the exception.

### Why freeze/thaw

"yelo" means ice in Tagalog. `freeze` and `thaw` are the natural vocabulary for archiving and restoring. They map to `put --storage-class DEEP_ARCHIVE` and `restore` respectively, hiding AWS jargon behind intuitive metaphors.

### Why a daemon

Glacier restores can take hours. The daemon polls pending restores and auto-downloads completed ones to `~/.yelo/cache/`. It runs as a standalone process (`yelo daemon start`) with PID file management, surviving TUI exits.
