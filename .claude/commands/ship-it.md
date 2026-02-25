Push changes, create a PR, review-fix until clean, and merge. End-to-end ship.

## Instructions

You are orchestrating the full ship workflow: push → PR → review-fix loop → merge. The argument $ARGUMENTS is optional context (e.g., a PR title hint or branch name).

### Step 1: Ensure we're on a feature branch

Check the current branch with `git branch --show-current`.

- If on `main`, you need to create a feature branch. Analyze the staged/unstaged changes and recent work to derive a good branch name (e.g., `feat/add-tests`, `fix/glacier-validation`). If `$ARGUMENTS` contains a branch name hint, use it. Then:
  1. Create and switch to the feature branch: `git checkout -b <branch-name>`
  2. Continue to Step 2.
- If already on a feature branch, continue.

### Step 2: Push changes and create the PR

1. Check for uncommitted changes with `git status`. If there are staged or unstaged changes (modified files, untracked files relevant to the current work):
   - Stage the relevant files (prefer `git add <specific files>` over `git add -A` to avoid accidentally including sensitive files or unrelated changes). Use the diff and file list to determine what's relevant.
   - Analyze the changes to write a clear commit message summarizing what was done.
   - Commit the changes.
   - Continue to step 2.

2. Push the branch to origin:
   ```
   git push -u origin $(git branch --show-current)
   ```

3. Check if a PR already exists for this branch:
   ```
   gh pr list --head $(git branch --show-current) --json number,url -q '.[0]'
   ```

4. If no PR exists, create one:
   - Gather the commit log for the PR body: `git log main..HEAD --oneline`
   - Create the PR:
     ```
     gh pr create --title "<descriptive title>" --body "$(cat <<'EOF'
     ## Summary
     <1-3 bullet points summarizing the changes based on commit history>

     ## Test plan
     - [ ] `go test ./...` passes
     - [ ] `go vet ./...` clean
     - [ ] Manual verification of changed functionality

     🤖 Generated with [Claude Code](https://claude.com/claude-code)
     EOF
     )"
     ```
   - If `$ARGUMENTS` is provided and looks like a title (not a number), use it as the PR title.
   - Otherwise, derive the title from the branch name and commits.

5. Store the PR number for subsequent steps.

### Step 3: Run the review-fix loop

#### Step 3a: Gather the diff

Fetch the diff: `gh pr diff <PR_NUMBER>`
Fetch the PR description: `gh pr view <PR_NUMBER>`

#### Step 3b: Identify changed files

List all files changed in the diff. Read the full content of each changed file (not just the diff hunks) so reviewers have complete context.

#### Step 3c: Launch parallel review agents

Launch ALL of the following review agents in parallel using the Task tool. Each agent should receive:
1. The full diff
2. The list of changed files
3. The full content of each changed file

**Agents to launch:**

1. **Architecture Review** (subagent_type: "general-purpose")
   - Prompt: Review these changes as an architecture reviewer. Check package boundaries (cmd/ wires things, internal/ packages are independent, no circular deps), module responsibilities, resolution chain correctness, and ripple effects. Refer to `CLAUDE.md` and `ARCHITECTURE.md` for the project's architectural principles. Here is the diff: [include diff]. Here are the full file contents: [include file contents]. Respond with LGTM if no issues, otherwise list issues as `[severity: high|medium|low] file:line — description`.

2. **Security Review** (subagent_type: "general-purpose")
   - Prompt: Review these changes as a security reviewer. Check for command injection, path traversal, credential exposure, unsafe file operations, and improper input validation. Since yelo interacts with AWS S3 and the local filesystem, pay special attention to key/path handling and config file safety. Here is the diff: [include diff]. Here are the full file contents: [include file contents]. Respond with LGTM if no issues, otherwise list issues as `[severity: high|medium|low] file:line — description`.

3. **Correctness Review** (subagent_type: "general-purpose")
   - Prompt: Review these changes for correctness. Check for logic errors, race conditions, resource leaks, error handling gaps (errors should wrap with context via fmt.Errorf), edge cases, and broken callers. Here is the diff: [include diff]. Here are the full file contents: [include file contents]. Respond with LGTM if no issues, otherwise list issues as `[severity: high|medium|low] file:line — description`.

4. **Code Quality Review** (subagent_type: "general-purpose")
   - Prompt: Review these changes for code quality. Check naming, complexity, duplication, dead code, error messages, consistency with adjacent code, and API design. Helpers should be pure functions, not methods on service objects. Output should respect TTY vs pipe conventions. Here is the diff: [include diff]. Here are the full file contents: [include file contents]. Respond with LGTM if no issues, otherwise list issues as `[severity: high|medium|low] file:line — description`.

5. **Vision Alignment Review** (subagent_type: "general-purpose")
   - Prompt: Review these changes for alignment with yelo's vision: choreography not orchestration (file-based coordination, no daemons), pipe-friendly (TTY gets human output, pipes get machine output, diagnostics to stderr), stdlib-only CLI parsing (no framework), and self-contained commands (read files → do work → write files). Check for feature creep, unnecessary dependencies, scope deviation, and complexity growth. Refer to `CLAUDE.md` for the project's conventions. Here is the diff: [include diff]. Here are the full file contents: [include file contents]. Respond with LGTM if no issues, otherwise list issues as `[severity: high|medium|low] — description`.

#### Step 3d: Compile and post the review

Once all agents complete, compile a unified review report in this format and post it as a comment on the PR using `gh pr comment`:

```
## PR Review: [PR title or branch name]

### Summary
[1-2 sentence summary of what the PR does]

### Architecture
[Agent findings or LGTM]

### Security
[Agent findings or LGTM]

### Correctness
[Agent findings or LGTM]

### Code Quality
[Agent findings or LGTM]

### Vision Alignment
[Agent findings or LGTM]

### Verdict
[APPROVE / REQUEST CHANGES / DISCUSS]
[1-2 sentence overall assessment]

### Issues by Severity
#### High
- [list all high severity issues across all reviewers, if any]

#### Medium
- [list all medium severity issues across all reviewers, if any]

#### Low
- [list all low severity issues across all reviewers, if any]
```

Use a HEREDOC to pass the review body:
```
gh pr comment <PR_NUMBER> --body "$(cat <<'REVIEW_EOF'
<compiled review markdown>
REVIEW_EOF
)"
```

If all reviewers say LGTM, the verdict is APPROVE.
If any reviewer has HIGH severity issues, the verdict is REQUEST CHANGES.
Otherwise, the verdict is DISCUSS.

#### Step 3e: Fix all issues

If the verdict is APPROVE (all LGTM), skip to Step 4.

Otherwise, fix every issue found by the reviewers — high, medium, and low. For each issue:
1. Read the relevant file(s) to understand the context
2. Make the fix using Edit/Write tools
3. Keep fixes minimal and focused — don't refactor beyond what the issue requires

After fixing all issues, run the test suite to make sure nothing is broken:
```
go test ./...
go vet ./...
```

If tests fail, fix them before proceeding.

Commit all fixes in a single commit with a message summarizing what was addressed:
```
fix: address review findings — [brief list of what was fixed]
```

Push the commit to the PR branch.

#### Step 3f: Re-review (loop)

Go back to Step 3a: gather the fresh diff, identify changed files, launch all 5 review agents again in parallel, compile the new review, and post it as a new comment on the PR.

**Keep looping Steps 3a–3f until the verdict is APPROVE** (all agents return LGTM or only have findings that are intentional/acknowledged).

To prevent infinite loops: if the same issue appears in 3 consecutive review rounds, stop the loop, post a comment explaining the unresolved issue, and ask the user for guidance.

### Step 4: Merge the PR

Once the review loop completes with APPROVE:

1. Post a final comment:
   ```
   gh pr comment <PR_NUMBER> --body "✅ All review agents report LGTM. Merging."
   ```

2. Merge the PR using squash merge to keep history clean:
   ```
   gh pr merge <PR_NUMBER> --squash --delete-branch
   ```

3. Switch back to main and pull:
   ```
   git checkout main && git pull
   ```

4. Tell the user the PR has been merged and the branch cleaned up. Include the PR URL in the final message.
