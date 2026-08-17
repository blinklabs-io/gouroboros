# Blink repository patterns

This document records reusable patterns found while bringing the managed Blink
Labs repositories into the workspace. It is intentionally a framework guide,
not a replacement for any project's local documentation.

## Workspace architecture

The workspace has three repository roles:

1. `repos/.github` supplies organization-wide community, contribution, and
   security defaults.
2. `repos/actions` supplies reusable GitHub Actions workflows and the
   organization governance engine.
3. The 27 project submodules contain applications, services, package
   definitions, and Docker images.

The parent repository pins source revisions for reproducible inspection and
cross-repository work. It does not merge the projects' histories or replace
their individual release processes.

## Project families

| Family | Projects | Shared workflow shape |
| --- | --- | --- |
| Standard Docker | 16 `docker-*` projects, excluding `docker-wireguard` | Conventional Commits, native multi-arch Docker CI, publish |
| Go/service | `adder`, `cardano-node-api`, `cardano-up`, `docker-wireguard`, `shai`, `tx-submit-api`, `tx-submit-api-mirror`, `txtop` | Conventional Commits, Go test/lint/NilAway, Docker CI, publish; individual projects may omit or add jobs |
| Mobile | `adder-mobile` | Conventional Commits plus Flutter/mobile-specific PR and publish workflows |
| Package definitions | `cardano-up-packages` | Conventional Commits, upstream version checks, package validation |
| Compose/integration | `cardano-compose-stacks` | Upstream version checks for a Docker Compose environment |

The authoritative profile and per-project exceptions live in
`repos/actions/repos-config.yaml`. Generated wrapper files in each project's
`.github/workflows/` should be treated as outputs of that configuration.

## Validation matrix

| Change | First checks | Follow-up checks |
| --- | --- | --- |
| Go source | Project `Makefile` target or `go test ./...` | `go vet`, `golangci-lint`, NilAway, nested-module tests |
| Generated OpenAPI or nested Go module | Format generated package and run its module tests | Root-module tests, lint, and NilAway |
| Dockerfile | `docker build --check .` | Full build, architecture matrix, and publish validation |
| GitHub workflow | `actionlint` | Review permissions, triggers, reusable workflow inputs, and generated-source ownership |
| Docker publish/tag logic | Inspect architecture-specific tags and manifest job | Validate tag and `latest` semantics in CI; do not push registries locally without authorization |
| Package definitions | Repository validation command and version consistency checks | Upstream release/version workflow |
| Parent submodule change | `git diff --check`, `git diff --submodule=log` | `git submodule status --recursive` and parent commit review |

Full Cardano/Haskell image builds can take a long time. Report when they are
not run rather than substituting a misleading partial result. Existing static
analysis findings should be identified by path and ownership before deciding
whether they are in scope.

## Governance implications

The `actions` governance engine can write repository settings, branch
protection, collaborators, and generated workflows directly to managed
repositories' default branches. This makes `repos-config.yaml` a high-impact
control-plane file: changes require focused review, correct GitHub App
permissions, and awareness that a scheduled sync can reconcile every managed
repository.

Downstream generated wrappers currently reference reusable workflows with
`blinklabs-io/actions/...@main`. Therefore, the submodule pointer here is a
local source snapshot, not a runtime version pin for consumers. A future
reproducibility/security improvement would be to consume release tags or commit
SHAs and define an update policy for them.

## Session-derived reusable knowledge

Local Codex sessions show recurring work patterns that are worth preserving:

- Review Go and generated OpenAPI changes in read-only issue-sized passes before
  editing, then validate nested modules before interpreting root lint results.
- Use worktrees or isolated branches for parallel fixes and keep pre-existing
  `.claude/`, `CLAUDE.md`, and other agent state untouched.
- Review Docker publish workflows as a complete pipeline: architecture image
  tags, manifest creation, release tags, and the conditions that update
  `latest` must agree.
- Prefer `docker build --check .` and `actionlint` before expensive builds or
  live registry validation.
- Separate merge blockers from non-blocking findings and explicitly report
  checks that require live GitHub, registry, or Cardano integration access.

These patterns are implemented first as the
[`blink-repo-maintainer`](../skills/blink-repo-maintainer/SKILL.md) skill. A
separate plugin is not warranted yet: the current findings are procedural and
local, while a plugin should add a concrete external integration or app
capability. Revisit a plugin once the team chooses a standard GitHub or issue
tracking integration for this workspace.
