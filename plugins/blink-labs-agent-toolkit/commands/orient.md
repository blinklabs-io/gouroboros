---
description: "Orient in the Blink Labs workspace: locate the owning repository, classify its family, and load the right skills before doing any work"
argument-hint: "[repository name, path, or topic]"
allowed-tools: ["Bash", "Glob", "Grep", "Read", "Task"]
---

# Orient in the Blink Labs workspace

Target: "$ARGUMENTS" (if empty, orient on the current working directory and
`git status --short`).

Do this before editing anything. The point is to know which repository owns the
change, which rules apply, and which checks are meaningful — not to start
fixing.

## Steps

1. **Find the boundary.** Run `git status --short` and `git rev-parse --show-toplevel`.
   Determine whether the target is the `clanker` workspace itself or a submodule
   under `repos/`. If the target is a name or topic rather than a path, search
   `repos/` for the owning project; use the `blink-workspace-navigator` skill's
   ownership map rather than guessing from a similar name.

2. **Read local instructions, in this order.** Workspace `AGENTS.md` and
   `CLAUDE.md`, then the target repository's `AGENTS.md`, `CLAUDE.md`,
   `CONTRIBUTING.md`, `README.md`, `Makefile`, and `CODEOWNERS`. Local
   instructions refine the workspace ones; where they conflict, the more local
   file wins for work inside that repository.

3. **Classify the family** using the `blink-repo-maintainer` skill and its
   `references/repository-families.md`. Report which family the repository
   belongs to and which validation profile follows from it.

4. **Enumerate module boundaries.** For Go repositories, list every nested
   `go.mod` (`find . -name go.mod -not -path './.git/*'`). State explicitly that
   a root `go test ./...` does not cover nested modules.

5. **Select the applicable skills** and say which ones you are loading:
   `cardano-protocol-reviewer`, `cardano-app-reviewer`, `dingo-maintainer`,
   `go-api-maintainer`, `go-dependency-auditor`, `docker-release-reviewer`,
   `infrastructure-reviewer`, `docs-kb-maintainer`,
   `github-review-coordinator`, `isolated-validation-runs`,
   `evidence-based-handoff`.

6. **Check workflow governance.** Inspect `.github/workflows/` in the target and
   its entry in `repos/actions/repos-config.yaml`. Generated wrapper workflows
   are outputs of the governance engine; note whether a workflow change belongs
   in the wrapper, the profile, or the reusable workflow.

## Report

Produce a short orientation brief: owning repository and path, family, module
list, the validation commands that apply, the skills loaded, and any local rule
that overrides a workspace default. Do not start implementation in this command
unless the user asked for it in the same message.
