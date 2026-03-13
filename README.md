# yelo

FTP-style CLI for Amazon S3 and Glacier. Navigate buckets with `cd`/`ls`/`pwd`, transfer with `get`/`put`, archive with `freeze`/`thaw`. Glacier-first — yelo means ice in Tagalog.

## Install

### Homebrew

```bash
brew install dorky-robot/tap/yelo
```

### Shell script

```bash
curl -fsSL https://raw.githubusercontent.com/dorky-robot/yelo/main/install.sh | bash
```

### From source

```bash
git clone https://github.com/dorky-robot/yelo.git
cd yelo
make install
```

### Docker

```bash
docker build -t yelo .
docker run --rm -v ~/.aws:/root/.aws yelo ls
```

## Quick start

```bash
# Add a bucket
yelo buckets add my-bucket --region us-east-1

# Navigate
yelo cd my-bucket:
yelo ls -l
yelo cd backups/2024/

# Freeze a file (archive to Glacier Deep Archive)
yelo freeze backup.tar.gz

# Thaw it when you need it back
yelo thaw backup.tar.gz

# Download once thawed
yelo get backup.tar.gz

# Regular upload/download
yelo put photo.jpg --storage-class STANDARD
yelo get report.csv
yelo get report.csv -          # stdout

# Interactive TUI
yelo tui
```

## TUI

Launch the interactive terminal UI with `yelo tui`. Four tabs:

| Tab | Key | Description |
|-----|-----|-------------|
| Browse | `1` | Navigate S3 buckets and objects |
| Profiles | `2` | Switch AWS profiles |
| Restores | `3` | Track Glacier restore requests |
| Library | `4` | Browse locally cached/downloaded files |

### Auto-download daemon

The daemon polls pending Glacier restores and downloads files automatically when they become available.

```bash
yelo daemon start     # start background polling
yelo daemon stop      # stop it
yelo daemon status    # check if running
```

You can also toggle the daemon from the TUI via the Restores tab submenu (`.` then `D`).

## CLI commands

```
yelo pwd                              Print working directory
yelo cd <path>                        Change directory (supports bucket:path)
yelo ls [-l] [-R] [path]              List objects/prefixes
yelo stat <key>                       Object metadata
yelo get <key> [dest|-]               Download (- for stdout)
yelo put <file> [key]                 Upload (default: DEEP_ARCHIVE)
yelo freeze <file> [key]              Archive to Glacier Deep Archive
yelo thaw <key> [--days N] [--tier T] Restore from Glacier
yelo restore <key> [--days N] [--tier T]  (alias for thaw)
yelo buckets [list|add|remove|default]    Manage buckets
yelo daemon [start|stop|status]           Background downloader
yelo tui                              Interactive terminal UI
```

### Global flags

| Flag | Description |
|------|-------------|
| `--bucket` | Override bucket resolution |
| `--region` | Override AWS region |
| `--profile` | Override AWS profile |

## Configuration

Config lives at `~/.config/yelo/config.yaml`:

```yaml
default_bucket: my-bucket
buckets:
  - name: my-bucket
    region: us-east-1
    profile: default
daemon:
  poll_interval: 60   # seconds between restore checks
```

State (current bucket/prefix) is stored at `~/.config/yelo/state.json`.

Cached files and restore notifications are stored under `~/.yelo/`.

## Resolution chains

Bucket, region, and profile resolve through priority-ordered fallback:

- **Bucket**: `--bucket` flag → state file → config default → sole configured bucket
- **Region**: `--region` flag → per-bucket config → global config → AWS SDK default
- **Profile**: `--profile` flag → per-bucket config → global config → AWS SDK default

## Building

```bash
make build          # debug build
make release        # optimized build
make test           # run tests
make lint           # clippy + rustfmt check
make install        # build + install to /usr/local/bin
make uninstall      # remove from /usr/local/bin
```

## Requirements

- Rust 1.85+ (edition 2024)
- AWS credentials configured (`~/.aws/credentials` or environment variables)

## License

MIT
