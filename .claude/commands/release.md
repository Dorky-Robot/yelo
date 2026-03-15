Cut a new release for yelo — bump version, tag, push, wait for CI, update Homebrew formulas, and verify the install.

## Step 1: Pre-flight checks

Verify the release environment is ready:

```bash
git branch --show-current
git status --porcelain
```

**Abort if:**
- Not on `main` — switch first or confirm with the user
- Working tree is dirty — commit or stash first

Pull latest to avoid conflicts:

```bash
git pull origin main
```

Show the current version:

```bash
grep '^version' Cargo.toml | head -1
```

## Step 2: Determine bump type

Check `$ARGUMENTS` for the bump type.

- If `$ARGUMENTS` contains `patch`, `minor`, `major`, or an explicit semver like `1.2.3`, use that.
- If `$ARGUMENTS` is empty or unclear, ask the user:
  - **patch** — bug fixes, docs, small tweaks
  - **minor** — new features, backward-compatible changes
  - **major** — breaking changes

Default to `patch` if empty.

## Step 3: Bump version

Edit `Cargo.toml` to set the new version using the Edit tool.

Read the new version and store it as `NEW_VERSION`.

Check if the tag already exists:

```bash
git tag -l "v$NEW_VERSION"
```

If the tag exists, revert the bump and stop. Tell the user to delete the existing tag or choose a different version.

## Step 4: Commit, tag, and push

```bash
git add Cargo.toml
git commit -m "release: v$NEW_VERSION"
git tag "v$NEW_VERSION"
git push origin main --tags
```

## Step 5: Wait for CI

The release workflow builds binaries for all platforms and creates a GitHub release. Wait for it:

```bash
gh run list --workflow=release.yml --limit=1 --json status,conclusion,databaseId
```

Poll until the run completes. If it fails, report the error and stop.

## Step 6: Compute SHA256 checksums

Download the release tarballs and compute checksums:

```bash
curl -sL --retry 5 --retry-delay 5 --retry-all-errors -f \
  "https://github.com/Dorky-Robot/yelo/releases/download/v${NEW_VERSION}/yelo-aarch64-apple-darwin.tar.gz" \
  -o "/tmp/yelo-mac-arm.tar.gz"

curl -sL --retry 5 --retry-delay 5 --retry-all-errors -f \
  "https://github.com/Dorky-Robot/yelo/releases/download/v${NEW_VERSION}/yelo-x86_64-apple-darwin.tar.gz" \
  -o "/tmp/yelo-mac-x86.tar.gz"

curl -sL --retry 5 --retry-delay 5 --retry-all-errors -f \
  "https://github.com/Dorky-Robot/yelo/releases/download/v${NEW_VERSION}/yelo-x86_64-unknown-linux-gnu.tar.gz" \
  -o "/tmp/yelo-linux.tar.gz"
```

Verify they're valid gzip:

```bash
file /tmp/yelo-mac-arm.tar.gz /tmp/yelo-mac-x86.tar.gz /tmp/yelo-linux.tar.gz
```

Compute the SHAs:

```bash
shasum -a 256 /tmp/yelo-mac-arm.tar.gz /tmp/yelo-mac-x86.tar.gz /tmp/yelo-linux.tar.gz
```

## Step 7: Update both formula files

### 7a: Local formula (`Formula/yelo.rb`)

Read the file, then update the `version`, `url`, and `sha256` lines using the Edit tool. There are three platform blocks to update (mac ARM, mac x86, linux).

Commit and push:

```bash
git pull origin main
git add Formula/yelo.rb
git commit -m "formula: update to v${NEW_VERSION}"
git push origin main
```

### 7b: Tap formula (`homebrew-tap/Formula/yelo.rb`)

The tap repo is `Dorky-Robot/homebrew-tap`. It should be cloned as a sibling directory. If not present:

```bash
cd .. && gh repo clone Dorky-Robot/homebrew-tap && cd -
```

Read the tap formula file, update `version`, `url`, and `sha256` with the same values.

Commit and push. **Note**: check the default branch:

```bash
cd ../homebrew-tap && git pull && git add Formula/yelo.rb && git commit -m "yelo: update to v${NEW_VERSION}" && git push && cd -
```

## Step 8: Brew upgrade

```bash
brew update
brew info dorky-robot/tap/yelo --json | jq -r '.[0] | "formula: \(.versions.stable)\ninstalled: \([.installed[].version] | join(","))"'
```

If installed version matches NEW_VERSION, `brew reinstall dorky-robot/tap/yelo`. Otherwise `brew upgrade dorky-robot/tap/yelo`.

## Step 9: Verify

1. Check CLI version:
   ```bash
   yelo --version
   ```

2. Clean up:
   ```bash
   rm -f /tmp/yelo-mac-arm.tar.gz /tmp/yelo-mac-x86.tar.gz /tmp/yelo-linux.tar.gz
   ```

3. Report:
   ```
   Released yelo v${NEW_VERSION}
   - Git tag: v${NEW_VERSION}
   - Formula: updated (local + tap)
   - Homebrew: installed and verified
   ```
