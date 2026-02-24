# CLAUDE.md — yelo

## Project overview

yelo is an FTP-style CLI for Amazon S3 and Glacier. Navigate buckets with `cd`/`ls`/`pwd`, transfer with `get`/`put`, manage archives with `restore`. Glacier-first — `put` defaults to DEEP_ARCHIVE. State persists between sessions via files on disk.

Module: `github.com/dorkyrobot/yelo`
Go 1.25.0, AWS SDK v2, no framework — stdlib `flag` for CLI parsing.

## Architecture

See `ARCHITECTURE.md` for full rationale. Key principles:

**Choreography, not orchestration.** Components coordinate through shared files, not a central controller. Config is a YAML file. State is a JSON file. Each command reads what it needs, does its job, writes its result. No daemon, no shared memory, no event bus.

**Resolution chains.** Bucket, region, and profile are resolved through priority-ordered fallback:
- Bucket: `--bucket` flag → state file → config default → sole configured bucket
- Region: `--region` flag → per-bucket config → global config → AWS SDK default
- Profile: `--profile` flag → per-bucket config → global config → AWS SDK default

**Pipe-friendly.** TTY gets human-readable output (columns, labels, progress bars). Pipes get machine-readable output (bare keys, tab-separated). Diagnostics always go to stderr.

## Code layout

```
main.go                         Entry point → cmd.Execute()
cmd/
  root.go                       Command registry, GlobalOpts, Env, resolution
  helpers.go                    Pure helpers (isGlacierClass, parseBucketPath)
  cd.go pwd.go ls.go            Navigation
  stat.go                       Object metadata
  get.go put.go                 Transfer (progress on stderr)
  restore.go                    Glacier restore with tier validation
  buckets.go                    Bucket config management (list/add/remove/default)
internal/
  config/config.go              YAML config at ~/.config/yelo/config.yaml
  state/state.go                JSON state at ~/.config/yelo/state.json
  state/resolve.go              FTP-style path resolution (pure functions)
  aws/client.go                 S3Client interface + NewClient
  aws/glacier.go                RestoreObject, ParseRestoreHeader, ValidateTier
  aws/transfer.go               Download/Upload with progress callbacks
  output/format.go              IsTTY, FormatSize, ListObjects, FormatStat
  tui/progress.go               Progress bar (stderr, nil on non-TTY)
doc/
  yelo.1                        Man page (roff)
```

## Package boundaries

- **cmd/** — wires everything together. Reads config/state, calls `internal/`, writes state, formats output. One file per command.
- **internal/config/** — YAML I/O. Knows nothing about AWS.
- **internal/state/** — JSON I/O + path resolution. Knows nothing about AWS.
- **internal/aws/** — SDK calls. Knows nothing about config files or output.
- **internal/output/** — TTY detection + formatting. Knows nothing about AWS.
- **internal/tui/** — Progress bars on stderr. Knows nothing else.

No circular dependencies. Each package takes data in, returns data out.

## Commands

```
yelo pwd                          bucket:/prefix/
yelo cd <path>                    FTP-style, supports bucket:path syntax
yelo ls [-l] [-R] [path]          List objects/prefixes
yelo stat <key>                   Object metadata
yelo get <key> [dest|-]           Download (- = stdout)
yelo put <file> [key]             Upload (default: DEEP_ARCHIVE)
yelo restore [--days N] [--tier T] <key>
yelo buckets [list|add|remove|default]
```

Global flags: `--bucket`, `--region`, `--profile`, `--config`

## Adding a new command

1. Create `cmd/<name>.go`
2. Write an `init()` that calls `register("<name>", runFunc, "usage string")`
3. The run function receives `(env *Env, args []string) error`
4. Use `env.ResolveBucket()` for bucket, `env.NewS3Client(ctx)` for AWS
5. Use `env.State` and `env.Cfg` for state/config access
6. Write output to stdout, diagnostics to stderr
7. If the command changes state, call `env.State.Save()`
8. Add the command to `printUsage()` in `root.go`
9. Add the command to `doc/yelo.1`

## Conventions

### Code style

- `go vet` and `go build` must pass
- No external CLI framework — stdlib `flag` only
- Commands are self-contained: read files → do work → write files
- Helpers are pure functions, not methods on service objects
- Errors wrap with context: `fmt.Errorf("doing thing: %w", err)`

### Output

- Human-readable on TTY, machine-readable when piped
- Progress bars and status messages → stderr
- Data → stdout
- `output.IsTTY()` to check, `output.FormatSize()` for human sizes
- `tui.NewProgress()` returns nil on non-TTY — safe to call unconditionally

### Glacier

- `isGlacierClass()` in helpers.go checks GLACIER, DEEP_ARCHIVE, GLACIER_IR
- `aws.ValidateTier()` blocks Expedited on DEEP_ARCHIVE
- `aws.ParseRestoreHeader()` interprets the x-amz-restore header
- `get` checks storage class before download, suggests `restore` if archived

### State and config

- Config: `~/.config/yelo/config.yaml` — buckets, defaults, region, profile
- State: `~/.config/yelo/state.json` — current bucket and prefix
- Both auto-create their directories on save
- State is the only mutable file during normal operation

### Man page

- `doc/yelo.1` is the canonical reference
- Validate with: `mandoc -Tlint doc/yelo.1`
- Preview with: `groff -man -Tutf8 doc/yelo.1 | less`
- Update when adding/changing commands or flags

## Building and testing

```bash
go build -o yelo .                # Build
go vet ./...                      # Lint
go mod tidy                       # Clean deps
./yelo help                       # Verify CLI runs
```

No test suite yet. When adding tests:
- `state/resolve.go` is pure logic — test path resolution first
- `internal/aws/glacier.go` has pure validation functions — test tier/class logic
- `internal/output/format.go` has pure formatting — test size and listing output
- Commands that call AWS should use the `S3Client` interface for mocking

## Part of the dorky robot stack

yelo is a standalone tool in the [Dorky Robot](https://github.com/Dorky-Robot) ecosystem. It shares architectural philosophy (file-based choreography, no daemons) with [sipag](https://github.com/Dorky-Robot/sipag) but has no runtime dependency on other projects.
