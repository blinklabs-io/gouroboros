---
description: "Orient in the Blink Labs and TosiDrop workspace: locate the owning organization and repository, classify its family, and load the right skills before doing any work"
argument-hint: "[repository name, path, or topic]"
allowed-tools: ["Bash", "Glob", "Grep", "Read", "Task"]
---

# Orient in the owned repositories

Target: "$ARGUMENTS" (if empty, orient on the current working directory and
`git status --short`).

Do this before editing anything. The point is to know which repository owns the
change, which rules apply, and which checks are meaningful — not to start
fixing.

## Steps

1. **Find the organization and boundary.** Run `git status --short`,
   `git rev-parse --show-toplevel`, and inspect the target's Git remote. For
   `blinklabs-io`, determine whether the target is `clanker` itself or a
   submodule under `repos/`; use `blink-workspace-navigator` instead of guessing
   from a similar name. For `TosiDrop`, use `tosidrop-repo-maintainer` and its
   repository map. TosiDrop repositories are standalone unless this checkout
   actually records one as a submodule.

2. **Read local instructions, in this order.** Workspace `AGENTS.md` and
   `CLAUDE.md`, then the target repository's `AGENTS.md`, `CLAUDE.md`,
   `CONTRIBUTING.md`, `README.md`, `Makefile`, and `CODEOWNERS`. Local
   instructions refine the workspace ones; where they conflict, the more local
   file wins for work inside that repository.

3. **Classify the family.** For `blinklabs-io`, use `blink-repo-maintainer` and
   `references/repository-families.md`. For `TosiDrop`, use
   `tosidrop-repo-maintainer` and `references/repository-map.md`. Report which
   validation profile and organization rules follow from that classification.

4. **Enumerate module boundaries.** For Go repositories, list every nested
   `go.mod` (`find . -name go.mod -not -path './.git/*'`). State explicitly that
   a root `go test ./...` does not cover nested modules.

5. **Select the applicable skills** and say which ones you are loading:
   `cardano-protocol-reviewer`, `cardano-app-reviewer`, `dingo-maintainer`,
   `go-api-maintainer`, `go-dependency-auditor`, `docker-release-reviewer`,
   `infrastructure-reviewer`, `blink-infrastructure-operator`,
   `blink-helm-chart-maintainer`, `blink-terraform-module-maintainer`,
   `blink-ansible-cardano-maintainer`, `docs-kb-maintainer`,
   `github-review-coordinator`, `isolated-validation-runs`,
   `evidence-based-handoff`.

6. **Check workflow governance.** Inspect `.github/workflows/` and live branch
   settings in the target. For Blink repositories, also inspect the entry in
   `repos/actions/repos-config.yaml`; generated wrappers are outputs of that
   governance engine. TosiDrop does not inherit the Blink actions configuration
   or organization-wide contribution policy.

## Report

Produce a short orientation brief: owning repository and path, family, module
list, the validation commands that apply, the skills loaded, and any local rule
that overrides a workspace default. Do not start implementation in this command
unless the user asked for it in the same message.
