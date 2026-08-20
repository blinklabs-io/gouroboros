# Agent instructions

This file contains repository-wide guidance for coding agents working in the
Blink Labs monorepo.

## Repository purpose

This repository coordinates Blink Labs projects and shared agentic-coding
assets. It contains Git submodules for every project alongside the shared
skills, plugin, documentation, and workspace-level automation that agents use
across them.

The canonical location for shared agent assets is
`plugins/blink-labs-agent-toolkit/`. The top-level `skills/` directory and the
`docs/go-repository-guide.md` and `docs/repository-patterns.md` files are
symlinks into it. Never replace a symlink with a copy; a second copy drifts
silently.

## Before making changes

1. Inspect the repository status and identify the requested change's scope.
2. Read this file and any more specific `AGENTS.md` files in the directory you
   will modify.
3. If the change is inside a Git submodule, read that project's contributor
   documentation and follow its build, test, and formatting instructions.
   `CLAUDE.md` provides the equivalent shared orientation for Claude-based
   work; Codex and review bots should use the same repository guide and local
   source-of-truth files.
4. Keep unrelated existing changes intact.

Planning notes and plan files are local, ephemeral working artifacts. Do not
commit them. When work needs durable tracking, create or update an issue with
the scope, acceptance criteria, and relevant context.

## Scope and repository boundaries

- Treat each submodule as an independently owned repository with its own Git
  history, tooling, and release process.
- Make source changes inside the relevant submodule, not in the monorepo around
  it. The parent repository should normally record only the resulting
  submodule pointer update and related workspace documentation.
- Do not rewrite, remove, or re-pin submodules unless the task explicitly asks
  for it.
- Shared skills, plugins, scripts, and documentation should be placed in their
  designated top-level directories as those directories are established.
- Before editing, find the owning repository with
  [`blink-workspace-navigator`](skills/blink-workspace-navigator/SKILL.md). Names
  are similar across families; a wrong guess costs a review cycle.
- For repository-aware work, use
  [`blink-repo-maintainer`](skills/blink-repo-maintainer/SKILL.md) and its
  repository-family reference.
- For Go repository work or code reviews, start with the shared
  [Go repository common ground](docs/go-repository-guide.md), then follow the
  target repository's local instructions. The guide maps module boundaries,
  generated interfaces, shared fixtures, review invariants, and repository
  pointers.
- For Dingo work, also read the [Dingo agent workflow](skills/dingo-maintainer/references/dingo-agent-workflow.md).
  Start from current `origin/main` in an isolated worktree, preserve live
  validation evidence, use unique ports and temporary paths, verify every
  background gate's output, and turn confirmed flakes or dropped events into
  issues rather than silently filtering them.
- Use the focused skills when their scope applies: [Dingo block-producer
  operator](skills/dingo-block-producer-operator/SKILL.md) for operating an
  existing producer, [Cardano protocol
  reviewer](skills/cardano-protocol-reviewer/SKILL.md), [Cardano application
  reviewer](skills/cardano-app-reviewer/SKILL.md), [Go API
  maintainer](skills/go-api-maintainer/SKILL.md), [Go dependency
  auditor](skills/go-dependency-auditor/SKILL.md), [Docker release
  reviewer](skills/docker-release-reviewer/SKILL.md), [infrastructure
  reviewer](skills/infrastructure-reviewer/SKILL.md), [docs and KB
  maintainer](skills/docs-kb-maintainer/SKILL.md), and [GitHub review
  coordinator](skills/github-review-coordinator/SKILL.md).
- Before changing a shape another component depends on — a response body, an
  error path, a public field, an ID format, or a persisted key — read the
  consumer first:
  [`cross-boundary-changes`](skills/cross-boundary-changes/SKILL.md). Fixing one
  side of a contract and shipping it is the most expensive mistake available
  here.
- When adding a test alongside a fix, prove it fails without the fix:
  [`regression-test-discipline`](skills/regression-test-discipline/SKILL.md).
- For process discipline, use
  [`isolated-validation-runs`](skills/isolated-validation-runs/SKILL.md) for
  slow, stateful, or concurrent checks,
  [`commit-and-pr-hygiene`](skills/commit-and-pr-hygiene/SKILL.md) before
  committing, and
  [`evidence-based-handoff`](skills/evidence-based-handoff/SKILL.md) before
  reporting work as done.
- The full catalog of skills, slash commands, subagents, and workspace guards is
  in [docs/skill-catalog.md](docs/skill-catalog.md). Changes to the toolkit
  itself follow
  [`agent-toolkit-authoring`](skills/agent-toolkit-authoring/SKILL.md).

## Implementation guidance

- Prefer small, focused changes that match the existing conventions.
- Avoid adding dependencies or workspace-wide automation without documenting
  why it belongs at the monorepo level.
- When a Docker image is available from `blinklabs-io`, prefer it over an
  upstream or third-party image. Record the reason for any exception.
- When Docker is available and useful for the requested check or reproduction,
  use it. Report Docker checks that were skipped because Docker was unavailable
  or the check was not materially useful.
- For source and Go dependencies, prefer the canonical upstream repository and
  module. Blink Labs forks are emergency-only exceptions and require explicit
  approval plus an issue documenting the reason and exit plan. Apollo is
  upstream-only under normal circumstances.
- Keep agent instructions clear, actionable, and narrowly scoped. More local
  instructions may refine or override these rules for their directory.
- Do not commit credentials, tokens, private configuration, build artifacts, or
  generated files unless the project explicitly tracks them.

## Commits and contributions

- Use [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/)
  for every commit. For example: `docs: clarify submodule workflow`.
- Do not include issue or pull-request numbers in commit subjects or bodies;
  reference issue tracking in the pull request or other review metadata.
- Keep commit subjects short and factual. If a body is needed, use only short
  factual lines directly tied to the changed code, tests, or review; do not use
  it for storytelling, chat transcripts, roadmaps, future plans, or unrelated
  context.
- Keep PR descriptions and review comments short, factual, and scoped to the
  current change. Put code-specific feedback in inline comments; reserve
  PR-level comments for concise summaries, checks, or dispositions.
- Useful Cubic and CodeRabbit summaries may remain in PR descriptions when they
  are clearly attributed and converted to plain Markdown. Remove generated
  HTML, buttons, hidden bot state, prompts, run IDs, stale commit metadata, and
  duplicate wrappers.
- UI changes must include screenshots in the pull request. Capture the affected
  states at the relevant viewport or platform and redact secrets or user data.
- Sign off commits with `git commit -s` to satisfy the Developer Certificate of
  Origin requirement.
- Read [`repos/.github/CONTRIBUTING.md`](repos/.github/CONTRIBUTING.md) and
  [`repos/.github/SECURITY.md`](repos/.github/SECURITY.md) for the current
  organization-wide contribution and security guidance.
- Check the relevant `CODEOWNERS` file and any local contribution instructions
  before preparing a pull request.
- Do not review draft pull requests. When the user excludes Dependabot, omit
  pull requests authored by `dependabot[bot]` from the review set.
- When asked to check for reviews or review requests, include pull requests
  assigned to any team the authenticated user belongs to, not only requests
  made directly to the user's account. If team memberships cannot be queried,
  report the scan as incomplete instead of claiming that no requests exist.
- Run the configured review bots before requesting human review and address
  their actionable findings first. If CodeRabbit is rate-limited, document it
  and a completed Cubic review is sufficient for bot review. Human review is
  still required; it may be AI-assisted, but bot approval or silence is not
  human approval.
- When asked to resolve an issue, carry it through implementation, PR update,
  bot review responses, valid fixes, validation, and bot re-runs until no
  actionable bot findings remain and the PR is ready for human review.
- When a human reviewer requests changes, implement and validate the fixes,
  summarize them on the pull request, and use GitHub to request another review
  from that same reviewer. Do not treat a reply or code update alone as a
  replacement for the requested follow-up review.
- An authorized human reviewer may dismiss another human review through
  GitHub when appropriate. Record the rationale on the pull request; dismissal
  does not remove the requirement for appropriate human review.
- Only the author merges their own pull request. The person who merges takes
  responsibility for the code, so approving a change is not the same as owning
  it — hand an approved PR back to its author. `dependabot[bot]` is the sole
  exception, since it cannot merge its own.
- A pull request may be squash-merged when GitHub shows human approval for the
  current head SHA, required checks pass, and configured bots have no
  actionable findings. Use one concise factual squash summary and preserve the
  DCO `Signed-off-by:` line.

## Validation

Run the narrowest relevant checks after making a change. For changes to a
submodule, use that submodule's documented checks. For workspace-level changes,
at minimum verify the resulting Git diff and, when applicable, validate the
affected Markdown, scripts, or plugin metadata.

For any change under `plugins/`, `skills/`, `docs/`, or `scripts/`, run
`make validate`. It checks manifest JSON, skill front matter, command and
subagent metadata, hook syntax and behavior, symlink integrity, and internal
link resolution.

When reporting results, include the checks that were run and note anything that
could not be run because the relevant project or tooling is not yet present.

## Git and submodules

When work changes a submodule:

1. Make and validate the change in the submodule repository.
2. Commit or otherwise preserve the submodule's intended revision according to
   the user's request.
3. Update and review the parent repository's submodule pointer.

Do not make a parent-repository commit on the user's behalf unless explicitly
asked. Keep submodule changes and parent-repository changes easy to distinguish
in the final summary.
