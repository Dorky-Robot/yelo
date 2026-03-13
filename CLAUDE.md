# CLAUDE.md — yelo

## Project overview

yelo is an FTP-style CLI for Amazon S3 and Glacier. Navigate buckets with `cd`/`ls`/`pwd`, transfer with `get`/`put`, manage archives with `freeze`/`thaw`. Glacier-first — `put` defaults to DEEP_ARCHIVE. State persists between sessions via files on disk.

Rust, AWS SDK v2, clap for CLI parsing, ratatui for TUI.

## Architecture

**Choreography, not orchestration.** Components coordinate through shared files, not a central controller. Config is a YAML file. State is a JSON file. Each command reads what it needs, does its job, writes its result.

**Resolution chains.** Bucket, region, and profile are resolved through priority-ordered fallback:
- Bucket: `--bucket` flag → state file → config default → sole configured bucket
- Region: `--region` flag → per-bucket config → global config → AWS SDK default
- Profile: `--profile` flag → per-bucket config → global config → AWS SDK default

**Pipe-friendly.** TTY gets human-readable output. Pipes get machine-readable output. Diagnostics go to stderr.

## Code layout

```
src/
  main.rs           Entry point, TUI event loop, key handlers
  cli.rs            Clap definitions, CLI command dispatch
  app.rs            TUI state (App struct, modes, background tasks)
  ui.rs             TUI rendering (ratatui)
  aws_ops.rs        S3/Glacier SDK calls (blocking tokio runtime)
  config.rs         YAML config at ~/.config/yelo/config.yaml
  state.rs          JSON state at ~/.config/yelo/state.json
  helpers.rs        Pure helpers (is_glacier, resolve_prefix, format_size)
  output.rs         TTY detection + CLI output formatting
  cache.rs          Local file cache at ~/.yelo/cache/
  restore.rs        Restore notifications at ~/.yelo/notifications/
  credentials.rs    AWS credentials INI management
  daemon.rs         Background auto-download daemon
  log.rs            File logging to ~/.yelo/daemon.log
```

## Module boundaries

- **cli.rs** — wires CLI commands. Reads config/state, calls aws_ops, writes state, formats output.
- **app.rs** — TUI state machine. Owns modes, background task channel, spawns threads.
- **ui.rs** — pure rendering. Reads App state, draws frames.
- **aws_ops.rs** — SDK calls. Knows nothing about config files or UI.
- **config.rs / state.rs** — file I/O. Know nothing about AWS.
- **helpers.rs** — pure functions shared by CLI and TUI.

No circular dependencies. Each module takes data in, returns data out.

## Commands

```
yelo pwd                              Print working directory
yelo cd <path>                        FTP-style, supports bucket:path syntax
yelo ls [-l] [-R] [path]              List objects/prefixes
yelo stat <key>                       Object metadata
yelo get <key> [dest|-]               Download (- = stdout)
yelo put <file> [key]                 Upload (default: DEEP_ARCHIVE)
yelo freeze <file> [key]              Archive to Glacier Deep Archive
yelo thaw <key> [--days N] [--tier T] Restore from Glacier
yelo restore <key> [--days N] [--tier T]  (same as thaw)
yelo buckets [list|add|remove|default]
yelo daemon [start|stop|status]
yelo tui                              Interactive terminal UI
```

Global flags: `--bucket`, `--region`, `--profile`

## TUI

Four tabs: Browse, Profiles, Restores, Library. Key handlers are in `main.rs`, rendering in `ui.rs`, state in `app.rs`.

Background tasks use `mpsc` channels — spawn a thread, send `BgResult` back, `poll_bg()` processes results each frame.

Modal overlays (confirm, forms, pickers) are `Mode` enum variants on `App`.

## Adding a new CLI command

1. Add a variant to `Command` enum in `cli.rs`
2. Add a match arm in `run()` in `cli.rs`
3. Write the run function, using `env.resolve_bucket()` etc.
4. Output to stdout, diagnostics to stderr

## Adding a new TUI action

1. If it needs a background task, add a `BgResult` variant and a `spawn_*` method on `App`
2. Handle the result in `poll_bg()` in `app.rs`
3. If it needs a modal, add a `Mode` variant
4. Add key handler in `main.rs`, rendering in `ui.rs`

## Conventions

### Code style

- `cargo clippy` and `cargo fmt` must pass (enforced by pre-push hook)
- Errors wrap with context: `anyhow::Context`
- Commands are self-contained: read files → do work → write files
- Helpers are pure functions

### Glacier

- `is_glacier()` checks GLACIER, DEEP_ARCHIVE, GLACIER_IR
- `validate_tier()` blocks Expedited on DEEP_ARCHIVE
- `get` checks storage class before download, suggests `restore`/`thaw` if archived

### State and config

- Config: `~/.config/yelo/config.yaml` — buckets, defaults, region, profile, daemon settings
- State: `~/.config/yelo/state.json` — current bucket and prefix
- Cache: `~/.yelo/cache/` — downloaded files
- Notifications: `~/.yelo/notifications/` — restore request tracking
- Daemon: `~/.yelo/daemon.pid`, `~/.yelo/daemon.log`

## Building and testing

```bash
make build        # debug build
make release      # optimized build
make test         # cargo test
make lint         # clippy + fmt check
make install      # build + install to /usr/local/bin
make setup        # configure git pre-push hook
```

Pre-push hook runs fmt, clippy, and tests before every push.

## Part of the dorky robot stack

yelo is a standalone tool in the [Dorky Robot](https://github.com/Dorky-Robot) ecosystem.
