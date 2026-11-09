# Agent Instructions

<!-- bd-doctor-divergence: ok -->

See [AGENT_INSTRUCTIONS.md](AGENT_INSTRUCTIONS.md) for full instructions.

This file exists for compatibility with tools that look for AGENTS.md.

The marker above tells `bd doctor` that the intentional divergence between
this file and `CLAUDE.md` (different audiences, different reading orders) is
expected and should not be flagged.

## Key Sections

- **Issue Tracking** - How to use bd for work management
- **Development Guidelines** - Code standards and testing
- **Visual Design System** - Status icons, colors, and semantic styling for CLI output
- **Contributor Protection** - Read [CONTRIBUTING.md](CONTRIBUTING.md) before handling external PRs
- **Maintainer PR Guidelines** - Read [PR_MAINTAINER_GUIDELINES.md](PR_MAINTAINER_GUIDELINES.md) before triaging, landing, or closing PRs

## PR Safety for Agents

Before triaging, reviewing, landing, closing, or otherwise maintaining PRs, read
[PR_MAINTAINER_GUIDELINES.md](PR_MAINTAINER_GUIDELINES.md). The maintainer
policy is to maximize community throughput: find useful contributor value,
absorb or transform it locally when practical, preserve attribution, and use
request-changes only as a last resort.

Before implementing work, opening a PR, or merging/closing a PR, run the PR
preflight:
```bash
scripts/pr-preflight.sh --search "<topic keywords>" --repo gastownhall/beads
scripts/pr-preflight.sh <pr-number> --repo gastownhall/beads
```

External contributor PRs have priority. Review and build on their branch when
possible, preserve their tests and attribution, and never close or supersede
their PR silently. If a rewrite is unavoidable, explain why on the original PR
and credit their design/tests.

## Visual Design Anti-Patterns

**NEVER use emoji-style icons** (🔴🟠🟡🔵⚪) in CLI output. They cause cognitive overload.

**ALWAYS use small Unicode symbols** with semantic colors:
- Status: `○ ◐ ● ✓ ❄`
- Priority: `● P0` (filled circle with color)

See [AGENT_INSTRUCTIONS.md](AGENT_INSTRUCTIONS.md) for full development guidelines.

## Storage Boundary

Beads talks to storage through a driver interface (`dolthub/driver` for Dolt).
Beads code should not reach across that boundary — no flocks, no engine
introspection, no storage-engine-specific retry or crash-recovery logic in
beads packages. Concurrency, locking, and crash-safety are the driver's job.

If you find yourself adding storage-implementation details to beads code —
especially anywhere on the public SDK surface (`OpenBestAvailable`, the
`Storage` interface, return types crossing `pkg/`) — stop and reconsider.
That is a signal the driver interface needs widening, not that beads needs
more storage logic.

**Roadmap target:** all storage interaction lives behind the driver. Beads
stays storage-agnostic.

**Filing storage issues is still encouraged** — but flag them clearly so
they can be routed to the driver (`dolthub/driver`) rather than patched
beads-side. Reflexive workarounds in beads (extra locks, retries, schema
poking) are exactly the kind of cross-boundary leak this direction is
removing. When in doubt, file the issue and ask which side owns the fix.

## Agent Warning: Interactive Commands

**DO NOT use `bd edit`** - it opens an interactive editor ($EDITOR) which AI agents cannot use.

Use `bd update` with flags instead:
```bash
bd update <id> --description "new description"
bd update <id> --title "new title"
bd update <id> --design "design notes"
bd update <id> --notes "additional notes"
bd update <id> --acceptance "acceptance criteria"

# Use stdin for descriptions with special characters (backticks, !, nested quotes)
echo 'Description with `backticks` and "quotes"' | bd create "Title" --description=-
echo 'Updated text' | bd update <id> --description=-
```

## Testing Commands (No Ambiguity)

- Default local test command: `make test` (or `./scripts/test.sh`).
- Opt-in ICU regex path: `make test-icu-path` (or `./scripts/test-icu-path.sh ./...`).
- This ICU path is maintainer-only and not part of normal validation; `make test-full-cgo` and `./scripts/test-cgo.sh` are deprecated aliases.
- For package- or test-scoped shipped-config CGO runs, prefer:
```bash
CGO_ENABLED=1 go test -tags gms_pure_go ./cmd/bd/...
CGO_ENABLED=1 go test -tags gms_pure_go -run '^TestName$' ./cmd/bd/...
```

## Non-Interactive Shell Commands

**ALWAYS use non-interactive flags** with file operations to avoid hanging on confirmation prompts.

Shell commands like `cp`, `mv`, and `rm` may be aliased to include `-i` (interactive) mode on some systems, causing the agent to hang indefinitely waiting for y/n input.

**Use these forms instead:**
```bash
# Force overwrite without prompting
cp -f source dest           # NOT: cp source dest
mv -f source dest           # NOT: mv source dest
rm -f file                  # NOT: rm file

# For recursive operations
rm -rf directory            # NOT: rm -r directory
cp -rf source dest          # NOT: cp -r source dest
```

**Other commands that may prompt:**
- `scp` - use `-o BatchMode=yes` for non-interactive
- `ssh` - use `-o BatchMode=yes` to fail instead of prompting
- `apt-get` - use `-y` flag
- `brew` - use `HOMEBREW_NO_AUTO_UPDATE=1` env var

## Landing the Plane (Session Completion)

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `git push` succeeds.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Create issues for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **PUSH TO REMOTE** - This is MANDATORY:
   ```bash
   git pull --rebase
   git push
   git status  # MUST show "up to date with origin"
   ```
5. **Clean up** - Clear stashes, prune remote branches
6. **Verify** - All changes committed AND pushed
7. **Hand off** - Provide context for next session

**CRITICAL RULES:**
- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds

<!-- BEGIN BEADS INTEGRATION v:1 profile:minimal hash:7510c1e2 -->
## Beads Issue Tracker

This project uses **bd (beads)** for issue tracking. Run `bd prime` to see full workflow context and commands.

### Quick Reference

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --claim  # Claim work
bd close <id>         # Complete work
```

### Rules

- Use `bd` for ALL task tracking — do NOT use TodoWrite, TaskCreate, or markdown TODO lists
- Run `bd prime` for detailed command reference and session close protocol
- Use `bd remember` for persistent knowledge — do NOT use MEMORY.md files

**Architecture in one line:** issues live in a local Dolt DB; sync uses `refs/dolt/data` on your git remote; `.beads/issues.jsonl` is a passive export. See https://github.com/gastownhall/beads/blob/main/docs/SYNC_CONCEPTS.md for details and anti-patterns.

## Session Completion

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `git push` succeeds.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Create issues for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **PUSH TO REMOTE** - This is MANDATORY:
   ```bash
   git pull --rebase
   git push
   git status  # MUST show "up to date with origin"
   ```
5. **Clean up** - Clear stashes, prune remote branches
6. **Verify** - All changes committed AND pushed
7. **Hand off** - Provide context for next session

**CRITICAL RULES:**
- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds
<!-- END BEADS INTEGRATION -->
