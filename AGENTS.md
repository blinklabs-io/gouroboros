# Agent instructions

This file contains repository-wide guidance for coding agents working in the
Blink Labs monorepo.

## Repository purpose

This repository coordinates Blink Labs projects and shared agentic-coding
assets. It is expected to contain Git submodules alongside skills, plugins,
documentation, and workspace-level automation.

## Before making changes

1. Inspect the repository status and identify the requested change's scope.
2. Read this file and any more specific `AGENTS.md` files in the directory you
   will modify.
3. If the change is inside a Git submodule, read that project's contributor
   documentation and follow its build, test, and formatting instructions.
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
- For repository-aware work, use the local
  [`blink-repo-maintainer`](skills/blink-repo-maintainer/SKILL.md) skill and
  its repository-family reference.

## Implementation guidance

- Prefer small, focused changes that match the existing conventions.
- Avoid adding dependencies or workspace-wide automation without documenting
  why it belongs at the monorepo level.
- When a Docker image is available from `blinklabs-io`, prefer it over an
  upstream or third-party image. Record the reason for any exception.
- Keep agent instructions clear, actionable, and narrowly scoped. More local
  instructions may refine or override these rules for their directory.
- Do not commit credentials, tokens, private configuration, build artifacts, or
  generated files unless the project explicitly tracks them.

## Commits and contributions

- Use [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/)
  for every commit. For example: `docs: clarify submodule workflow`.
- Sign off commits with `git commit -s` to satisfy the Developer Certificate of
  Origin requirement.
- Read [`repos/.github/CONTRIBUTING.md`](repos/.github/CONTRIBUTING.md) and
  [`repos/.github/SECURITY.md`](repos/.github/SECURITY.md) for the current
  organization-wide contribution and security guidance.
- Check the relevant `CODEOWNERS` file and any local contribution instructions
  before preparing a pull request.

## Validation

Run the narrowest relevant checks after making a change. For changes to a
submodule, use that submodule's documented checks. For workspace-level changes,
at minimum verify the resulting Git diff and, when applicable, validate the
affected Markdown, scripts, or plugin metadata.

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
