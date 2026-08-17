# Blink repository patterns

This document records reusable patterns found while bringing the managed Blink
Labs repositories into the workspace. It is intentionally a framework guide,
not a replacement for any project's local documentation.

## Workspace architecture

The workspace has five repository roles:

1. `repos/.github` supplies organization-wide community, contribution, and
   security defaults.
2. `repos/actions` supplies reusable GitHub Actions workflows and the
   organization governance engine.
3. `repos/docs` publishes user-facing product and DevOps documentation at
   `docs.blinklabs.io`.
4. `repos/kb` contains long-form developer training and engineering reference
   books, with a policy of linking published material to pinned public sources.
5. The 27 managed project submodules contain applications, services, package
   definitions, and Docker images.
6. Additional ecosystem submodules contain protocol libraries, deployment
   automation, infrastructure modules, Helm charts, and shared issue context.

The parent repository pins source revisions for reproducible inspection and
cross-repository work. It does not merge the projects' histories or replace
their individual release processes.

The governance engine currently covers the 27 application, service, package,
and container repositories in `repos/actions/repos-config.yaml`. The `docs`
and `kb` submodules are not in that configuration and currently have no
repository-local GitHub workflows. Treat that as an intentional coverage gap
until documentation-specific profiles and checks are defined.

## Project families

| Family | Projects | Shared workflow shape |
| --- | --- | --- |
| Standard Docker | 16 `docker-*` projects, excluding `docker-wireguard` | Conventional Commits, native multi-arch Docker CI, publish |
| Go/service/library | `adder`, `bark`, `bluefin`, `bursa`, `cardano-models`, `cardano-node-api`, `cardano-up`, `dingo`, `docker-wireguard`, `go-bip39`, `go-scls`, `gouroboros`, `nview`, `ouroboros-mock`, `plutigo`, `shai`, `tx-submit-api`, `tx-submit-api-mirror`, `txtop` | Go tests, lint/NilAway, and publishing where configured; protocol repositories may add Buf, fuzz, benchmark, or generated-code checks |
| Mobile | `adder-mobile` | Conventional Commits plus Flutter/mobile-specific PR and publish workflows |
| Package definitions | `cardano-up-packages` | Conventional Commits, upstream version checks, package validation |
| Compose/integration | `cardano-compose-stacks` | Upstream version checks for a Docker Compose environment |
| Public documentation | `docs` | Next.js/Nextra site with MDX product and DevOps documentation |
| Engineering knowledge base | `kb` | Numbered training books with references, glossaries, and source maps |
| Ansible automation | `ansible-cardano` | Ansible Galaxy collection with role tests and release workflow |
| Helm packaging | `helm-charts` | Many chart-specific publish workflows plus chart testing and image-version checks |
| Terraform infrastructure | `terraform-modules` | Terraform module validation and release workflows |
| Shared issue context | `issues` | Content-only repository with no build workflow |

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
| Go protocol/library | `go test ./...` and repository `Makefile` targets | `buf lint/generate`, fuzzing, benchmarks, NilAway, or generated-code checks as applicable |
| Ansible collection | `ansible-test` for affected roles | `ansible-lint` and release packaging |
| Helm chart | `helm lint` and `helm template` for affected charts | chart-testing and registry/release workflow |
| Terraform module | `terraform fmt -check` and `terraform validate` per module | provider-aware plan or integration checks |
| Issue/content repository | Review Markdown and repository links | No code build unless the repository adds one |
| Public documentation | `pnpm install --frozen-lockfile`, then `pnpm build` | Review MDX links and update `pages/_meta.json` when adding navigation entries |
| Knowledge base | Check book structure, Markdown links, and pinned source URLs | Run a link checker when available; preserve each book's README, start page, glossary, and source map |
| Parent submodule change | `git diff --check`, `git diff --submodule=log` | `git submodule status --recursive` and parent commit review |

Full Cardano/Haskell image builds can take a long time. Report when they are
not run rather than substituting a misleading partial result. Existing static
analysis findings should be identified by path and ownership before deciding
whether they are in scope.

For Docker dependencies, prefer `blinklabs-io` images whenever an equivalent
image is available. Verify the image's tag, architecture support, and source
before using it; document any intentional upstream or third-party fallback.

The current audit found a concrete follow-up in `cardano-compose-stacks`:
Kupo and Ogmios use third-party image names even though Blink Labs maintains
`docker-kupo` and `docker-ogmios`. Track that as an issue and update it only
after confirming tag compatibility and the desired release policy.

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

## Documentation boundaries

Use `docs` for concise, navigable public documentation: installation,
configuration, quickstarts, product concepts, and DevOps operation. Use `kb`
for durable onboarding and deep technical learning: architecture walkthroughs,
protocol primers, package maps, debugging, testing, and contribution context.

When documenting a project, prefer linking between the public site, the
appropriate knowledge-base book, and the project's README rather than copying
large explanations into multiple locations. Preserve `kb`'s pinned-source-link
policy so training examples remain reproducible.

The docs site currently uses Next.js/Nextra with 41 MDX pages and `pnpm`
scripts, while the knowledge base is Markdown-only with five numbered books
and no build manifest. The audit also found that `kb` has no root `LICENSE` or
`CONTRIBUTING.md`; track explicit licensing and contribution metadata as a
separate issue rather than assuming the parent repository's license applies.

## Session-derived reusable knowledge

Local Codex sessions show recurring work patterns that are worth preserving:

- Review Go and generated OpenAPI changes in read-only issue-sized passes before
  editing, then validate nested modules before interpreting root lint results.
- Use worktrees or isolated branches for parallel fixes and keep pre-existing
  `.claude/`, `CLAUDE.md`, and other agent state untouched.
- Review Docker publish workflows as a complete pipeline: architecture image
  tags, manifest creation, release tags, and the conditions that update
  `latest` must agree.
- For Dingo, treat race-enabled tests, architecture-boundary checks, shared
  `ouroboros-mock` fixtures, Cardano conformance/devnet validation, and
  `DATABASE.md`/`ARCHITECTURE.md` review as one change bar for node-level work.
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
