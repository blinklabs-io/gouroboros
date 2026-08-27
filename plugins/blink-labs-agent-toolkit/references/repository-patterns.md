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
3. `repos/docs-site` publishes user-facing product and DevOps documentation at
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
| Go/service/library | `adder`, `bark`, `bluefin`, `bursa`, `cardano-models`, `cardano-node-api`, `cardano-up`, `dingo`, `docker-wireguard`, `go-bip39`, `go-scls`, `gouroboros`, `merkle-patricia-forestry`, `nview`, `ouroboros-mock`, `plutigo`, `shai`, `tx-submit-api`, `tx-submit-api-mirror`, `txtop` | Go tests, lint/NilAway, and publishing where configured; protocol repositories may add Buf, fuzz, benchmark, or generated-code checks |
| Mobile | `adder-mobile` | Conventional Commits plus Flutter/mobile-specific PR and publish workflows |
| Package definitions | `cardano-up-packages` | Conventional Commits, upstream version checks, package validation |
| Compose/integration | `cardano-compose-stacks` | Upstream version checks for a Docker Compose environment |
| Corporate website | `www` | React/Vite/TypeScript build, tests, lint, formatting, prerendering, and public-link health |
| Public documentation | `docs-site` | Astro/Starlight site with Markdown and localized product and DevOps documentation |
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
| React/Vite website | `npm ci`, typecheck, ESLint, Prettier, build, and tests | Run the repository's public-link checker for URL changes; capture screenshots only when rendered behavior changes |
| Issue/content repository | Review Markdown and repository links | No code build unless the repository adds one |
| Public documentation | `npm ci`, then `npm run check` and `npm run build` | Review Markdown links and update Starlight navigation when adding pages under `src/content/docs/` |
| Knowledge base | Check book structure, Markdown links, and pinned source URLs | Run a link checker when available; preserve each book's README, start page, glossary, and source map |
| Parent submodule change | `git diff --check`, `git diff --submodule=log` | `git submodule status --recursive` and parent commit review |

## Release publication rule

Across all Blink Labs repositories, agents create tags only. Never create a
GitHub release object manually with `gh release create`, the Releases API, or
an equivalent command. Verify the repository workflow and its consumer-visible
artifact after tagging; if automated publication fails or reports an immutable
release record, stop and hand recovery to the owner rather than deleting,
recreating, or replacing the tag or release.

**Never manually release. Ever.** Release objects and package publication are
owned by repository automation or the owner.

Full Cardano/Haskell image builds can take a long time. Report when they are
not run rather than substituting a misleading partial result. Existing static
analysis findings should be identified by path and ownership before deciding
whether they are in scope.

For Docker dependencies, prefer `blinklabs-io` images whenever an equivalent
image is available. Verify the image's tag, architecture support, and source
before using it; document any intentional upstream or third-party fallback.

For source and Go dependencies, use the canonical upstream repository and
module. Blink Labs forks are emergency-only exceptions that require explicit
approval, an issue, and an exit plan. Apollo is upstream-only under normal
circumstances; use the external upstream module at `Salvionied/apollo` rather
than adding it to this workspace.

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

The active docs site at `repos/docs-site` is the `blinklabs-io/docs`
repository. It uses Astro/Starlight, npm scripts, and localized content under
`src/content/docs/`; it is not the archived `blinklabs-io/docs-site` repository.
The knowledge base is Markdown-only with five numbered books and no build
manifest. The audit also found that `kb` has no root `LICENSE` or
`CONTRIBUTING.md`; track explicit licensing and contribution metadata as a
separate issue rather than assuming the parent repository's license applies.

## Session-derived reusable knowledge

Local Codex sessions show recurring work patterns that are worth preserving:

- Review Go and generated OpenAPI changes in read-only issue-sized passes before
  editing, then validate nested modules before interpreting root lint results.
- Validate a Go dependency update against the complete selected graph with
  `GOWORK=off`, not only the direct module named by the change. If a transitive
  fixture update changes conformance behavior, isolate old/new direct and
  transitive pairs before assigning ownership; never pin a stale fixture solely
  to make the consumer green.
- Keep regression failures small enough to review. Aggregate counters, bytes,
  or allocations and assert once after a large workload instead of emitting a
  full value diff on every iteration of the fail-before run.
- Use worktrees or isolated branches for parallel fixes and keep pre-existing
  `.claude/`, `CLAUDE.md`, and other agent state untouched.
- Review Docker publish workflows as a complete pipeline: architecture image
  tags, manifest creation, release tags, and the conditions that update
  `latest` must agree.
- For Dingo, treat race-enabled tests, architecture-boundary checks, shared
  `ouroboros-mock` fixtures, Cardano conformance/devnet validation, and
  `DATABASE.md`/`ARCHITECTURE.md` review as one change bar for node-level work.
- Dingo investigations should begin from current `origin/main` in an isolated
  worktree, preserve live-run evidence, use unique ports and temporary paths,
  and verify the complete output of every background gate. Confirmed flakes,
  dropped events, and infrastructure limitations belong in issues rather than
  filtered logs or committed plan files.
- Dingo review findings require control-flow validation and a focused test for
  the negative case: eligibility, equal-height selection, lifecycle
  cancellation, mutually exclusive flags, gate absence, and script argument or
  port handling are recurring failure surfaces.
- When Docker is available and useful, prefer `docker build --check .` and a
  targeted local reproduction before expensive builds or live registry
  validation. Report when Docker is unavailable or not useful for the check.
- Separate merge blockers from non-blocking findings and explicitly report
  checks that require live GitHub, registry, or Cardano integration access.
- Run CodeRabbit and Cubic before human review, address actionable findings,
  and then obtain the required human review. If CodeRabbit is rate-limited,
  document it; a completed Cubic review is sufficient for bot review. Human
  review may be AI-assisted, but bot approval is not a substitute for it.
- If a human reviewer requests changes, validate and push the fixes, summarize
  them on the pull request, and use GitHub to request another review from that
  same reviewer.
- An authorized human reviewer may dismiss another human review through
  GitHub when appropriate; record the rationale on the pull request and retain
  the required human-review coverage.
- Only the author merges their own pull request; the person who merges takes
  responsibility for the code. `dependabot[bot]` is the sole exception.
- Squash merge is allowed only with a human approval attached to the current PR
  head, passing required checks, and no actionable bot findings. Use one concise
  factual squash summary and preserve the DCO `Signed-off-by:` line. A review
  for an earlier head is stale after a push.
- Issue resolution includes the bot loop after the initial PR update: verify
  CodeRabbit and Cubic findings against the current head, or document CodeRabbit
  rate limiting and use Cubic alone, fix valid findings, validate, and repeat
  until the available bots are clear for human review.
- Keep commits, PR descriptions, and reviews concise and evidence-based. Use
  short factual statements tied to the changed code, tests, and review state;
  place code-specific feedback inline and keep PR-level text to summaries,
  checks, and dispositions.
- Review current PR heads, not stale bot or human findings. Skip drafts and
  apply an explicit Dependabot exclusion before reading diffs.
- Keep static website cleanup proportional to the behavior changed. Trace data
  consumers, preserve valid customer or partner records, and use existing
  type, build, and link checks when rendered behavior is unchanged. Do not add
  tests that merely forbid a deleted literal from returning; reserve new
  regression coverage for durable behavior or contracts.
- UI changes are not review-complete without screenshots of the affected states
  in the PR at the relevant viewport or platform; redact secrets and user data.
- Go API reviews must cover value-versus-pointer serialization behavior and
  typed-nil type switches. Confirm the module Go version before accepting
  range-variable alias findings.
- Generated workflow wrappers must resolve to files in the current
  `repos/actions` source. Mutable `@main` refs on release or secret-bearing
  calls and broad caller permissions are merge blockers.
- Dingo cache reviews must cover hot-read TTLs, retained-object bounds,
  in-flight waiter and panic cleanup, and benchmark worker error handling.
  Koios/account reviews must cover exact amount validation, duplicate rows,
  coverage errors, composition defaults, and first-failure cancellation.
- After bounding retained memory through compact or lazy representation, review
  the neighboring recovery path for repeated decode and scan cost. Memory and
  CPU controls can be separate changes, but both costs must remain bounded and
  their dependency explicit.
- Treat host disk recovery as a reviewed artifact cleanup: measure before and
  after, select exact cache paths, and exclude symlinks, mountpoints, registered
  or dirty worktrees, live process paths, container mounts, and retained
  evidence. Inspect descendant ownership because a host-owned cache root can
  contain files written by a root-running container.
- Inspect Docker's default builder and every named Buildx builder separately;
  `docker system df` does not necessarily account for `docker-container` builder
  cache. Prune only idle build cache by default. Reclaimable images, volumes,
  networks, and containers need separate run ownership and evidence checks.
- A merged temporary branch is safe to delete only after its exact head is an
  ancestor of `origin/main` or present in the merged pull request. This matters
  in submodules whose primary checkout is detached or behind the remote: after
  the explicit ancestry proof, remove the clean worktree through Git and delete
  only that local branch, preserving dirty trees and remote refs.

These patterns are implemented by the
[`blink-repo-maintainer`](../skills/blink-repo-maintainer/SKILL.md) skill, with
focused companion skills for [Cardano protocol review](../skills/cardano-protocol-reviewer/SKILL.md),
[Go API maintenance](../skills/go-api-maintainer/SKILL.md), [Go dependency
auditing](../skills/go-dependency-auditor/SKILL.md), [Docker release
review](../skills/docker-release-reviewer/SKILL.md), [Cardano application
review](../skills/cardano-app-reviewer/SKILL.md), [docs and KB
maintenance](../skills/docs-kb-maintainer/SKILL.md), and [GitHub review
coordination](../skills/github-review-coordinator/SKILL.md), plus
[isolated validation runs](../skills/isolated-validation-runs/SKILL.md) for
worktree and host-resource lifecycle. A separate plugin
is not warranted yet: the current findings are procedural and local, while a
plugin should add a concrete external integration or app capability. Revisit a
plugin once the team chooses a standard GitHub or issue tracking integration
for this workspace.

For Go-specific orientation, module boundaries, review invariants, generated
API pointers, and the dependency relationship between protocol libraries and
applications, use the [Go repository common-ground guide](go-repository-guide.md).

Apollo is an external dependency with the upstream
`github.com/Salvionied/apollo/v2` module path. Shai is transitioning back to
that upstream dependency; do not add a parent-workspace replacement or rewrite
its module metadata while that transition is in flight.
