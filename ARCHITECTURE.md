# yelo — Architecture

## What yelo is

yelo is an FTP-style CLI for Amazon S3 and Glacier. You `cd` into buckets, `ls` prefixes, `get` and `put` objects, `restore` archives. It remembers where you are between sessions.

## Architectural principles

### Choreography over orchestration

No central controller. Components coordinate through shared files and conventions, not through a conductor that calls them in sequence. Each piece reads what it needs, does its job, writes its result. The filesystem is the message bus.

This means:

- **Config is a file** (`~/.config/yelo/config.yaml`). Any tool can read or write it — yelo, a script, a text editor, another CLI. No API, no daemon, no lock.
- **State is a file** (`~/.config/yelo/state.json`). The current bucket and prefix persist between sessions as a JSON blob. `cd` writes it; `ls`, `get`, `pwd` read it. They don't talk to each other — they talk to the file.
- **Commands are independent**. Each command resolves its own context (bucket, prefix, region, profile) by reading config and state. There's no shared in-memory state, no command bus, no event system. A command starts, reads files, calls AWS, writes files, exits.

### Files as the coordination layer

Following the same pattern as [sipag](https://github.com/Dorky-Robot/sipag):

| sipag | yelo | Principle |
|---|---|---|
| `queue/*.md` task files | `~/.config/yelo/state.json` | Filesystem is the database |
| Task files are plain markdown | Config is YAML, state is JSON | Human-readable, debuggable with `cat` |
| TUI and executor share a directory | All commands share config + state files | Decoupled components, shared surface |
| Any tool can drop a `.md` in `queue/` | Any tool can edit config or state | Open for composition |
| `ls running/` tells you what's happening | `cat state.json` tells you where you are | Inspectable without special tooling |

Why files, not an in-memory registry or database:

- **Any tool can participate.** A shell script can set the bucket by writing `state.json`. Another CLI can add a bucket by appending to `config.yaml`. Composition is free.
- **Human-readable.** `cat`, `vim`, `jq`. No query language, no API.
- **Debuggable.** When something is wrong, you read the file. No hidden state.
- **Crash-proof.** If yelo dies mid-command, the last-written state is still there. No in-memory state to lose.
- **No daemon.** yelo is not a long-running process. Each invocation is stateless except for what it reads from disk.

### Resolution chains, not configuration hierarchies

When a command needs a value (bucket, region, profile), it walks a resolution chain — a priority-ordered list of places to look. The first source that has an answer wins.

**Bucket resolution:**
```
flag (--bucket) → state file → config default → sole configured bucket
```

**Region resolution:**
```
flag (--region) → per-bucket config → global config → AWS SDK default
```

**Profile resolution:**
```
flag (--profile) → per-bucket config → global config → AWS SDK default
```

This is choreography at the value level. No single source of truth — instead, a clear priority order that any component can participate in. A CI script can set `--bucket` as a flag. An interactive user can `cd` into a bucket (which writes state). A global default covers the common case. They all work, and the resolution chain decides who wins.

### Pipe-friendly by default

yelo adapts its output based on whether stdout is a TTY:

| Context | Behavior |
|---|---|
| Interactive terminal | Human-readable: aligned columns, labels, progress bars |
| Piped / redirected | Machine-readable: bare keys, tab-separated values, no progress noise |

All diagnostic output (progress bars, status messages) goes to stderr. Data goes to stdout. This means `yelo get myfile.tar.gz -` pipes cleanly, and `yelo ls | grep pattern` works without filtering out progress bars.

This is another form of choreography — yelo doesn't need to know what's consuming its output. It checks the file descriptor and adapts. The downstream tool doesn't ask yelo to be quiet; yelo observes its environment and responds.

### Commands are self-contained

Each command is a single function that:

1. Reads config and state from files
2. Resolves its context (bucket, prefix, region, profile) through the resolution chain
3. Calls AWS
4. Writes any state changes back to files
5. Outputs results

No shared runtime, no middleware, no command pipeline. Commands don't depend on each other at runtime — they depend on the files. This is what makes them composable: `yelo cd mybucket:/data && yelo ls -l` works because `cd` writes state and `ls` reads it. They coordinate through the filesystem, not through a shared process.

### Glacier awareness as a cross-cutting concern

Glacier storage classes (GLACIER, DEEP_ARCHIVE, GLACIER_IR) affect multiple commands differently:

- `get` checks storage class before attempting download, suggests `restore` if archived
- `put` defaults to DEEP_ARCHIVE (yelo is Glacier-first)
- `restore` validates tier against storage class (no Expedited for DEEP_ARCHIVE)
- `stat` shows restore status
- `ls -l` shows storage class

This isn't implemented as middleware or a shared concern — each command handles Glacier in its own way, using shared helper functions (`isGlacierClass`, `ParseRestoreHeader`, `ValidateTier`). The helpers are pure functions, not services. They take data and return data.

## File layout

```
~/.config/yelo/
  config.yaml          # Buckets, defaults, region, profile
  state.json           # Current bucket and prefix
```

### config.yaml

```yaml
default: my-archive
buckets:
  my-archive:
    region: us-west-2
    profile: personal
  work-backup:
    region: us-east-1
region: us-west-2
profile: default
```

### state.json

```json
{
  "bucket": "my-archive",
  "prefix": "photos/2024/"
}
```

## Command structure

```
yelo
  pwd                 Show current bucket and prefix
  cd <path>           Change working directory (FTP-style)
  ls [path]           List objects and prefixes
  stat <key>          Show object metadata
  get <key> [dest]    Download object (- for stdout)
  put <file> [key]    Upload object (default: DEEP_ARCHIVE)
  restore <key>       Initiate Glacier restore
  buckets             Manage configured buckets
    (no subcommand)   List buckets
    add <name>        Add a bucket
    remove <name>     Remove a bucket
    default [name]    Get or set default bucket
```

## Internal structure

```
yelo/
  main.go                       Entry point
  go.mod
  cmd/
    root.go                     Command registry, global flags, env resolution
    helpers.go                  Shared pure functions
    cd.go, pwd.go, ls.go        Navigation commands
    stat.go                     Metadata command
    get.go, put.go              Transfer commands
    restore.go                  Glacier restore command
    buckets.go                  Bucket management command
  internal/
    config/config.go            YAML config read/write
    state/
      state.go                  JSON state read/write
      resolve.go                FTP-style path resolution (pure logic)
    aws/
      client.go                 S3Client interface, initialization
      glacier.go                Restore operations, tier validation
      transfer.go               Download/upload with progress
    output/format.go            TTY detection, adaptive formatting
    tui/progress.go             Terminal progress bars (stderr)
  doc/
    yelo.1                      Man page
```

### Package boundaries

- **cmd/** — Command definitions. Each file registers one command. Commands read config/state, call into `internal/`, write state, format output.
- **internal/config/** — Config file I/O. Knows about YAML. Doesn't know about AWS or commands.
- **internal/state/** — State file I/O and path resolution. Knows about JSON and FTP-style paths. Doesn't know about AWS or commands.
- **internal/aws/** — AWS operations. Knows about the SDK. Doesn't know about config files, state files, or output formatting.
- **internal/output/** — Output formatting. Knows about TTY detection and column alignment. Doesn't know about AWS or state.
- **internal/tui/** — Progress bars. Knows about terminal width and stderr. Doesn't know about anything else.

Each package reads data in and pushes data out. No circular dependencies. No package reaches into another's internals.

## Design decisions

### Why FTP-style navigation

S3 is a flat key-value store, but humans think in directories. FTP-style `cd`/`ls`/`pwd` maps the mental model people already have onto S3's prefix-based listing. The state file makes this work across invocations — you don't lose your place.

### Why Glacier-first

yelo defaults `put` to DEEP_ARCHIVE because the primary use case is cold archival. Most S3 CLIs default to STANDARD and treat Glacier as an afterthought. yelo treats Glacier as the default and STANDARD as the exception.

### Why no `rm`, `cp`, `mv` (yet)

Destructive operations on archival storage deserve careful thought. `rm` on DEEP_ARCHIVE is irreversible in a way that `rm` on STANDARD is not (no versioning safety net in typical archival setups). These commands should come with appropriate guardrails, not as afterthoughts.

### Why no daemon

A daemon would introduce coordination problems (is it running? which port? how do I restart it?) for no benefit. yelo commands are short-lived and fast. The filesystem provides all the persistence needed.
