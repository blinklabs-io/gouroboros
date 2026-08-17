---
name: blink-repo-maintainer
description: Inspect, classify, and validate Blink Labs repositories and submodules using organization conventions, generated GitHub Actions workflows, Docker/Go/Cardano project patterns, Conventional Commits, and DCO. Use when working in a Blink Labs repository, adding or updating a repository in the clanker workspace, reviewing a project change, or deciding which checks and shared workflow profile apply.
---

# Blink Repository Maintainer

Use this skill to make repository-aware changes across the Blink Labs project
family. Treat the parent `clanker` repository as a workspace and each entry
under `repos/` as an independently versioned project.

For Go repositories or cross-repository code reviews, read the shared
[Go repository common-ground guide](../../docs/go-repository-guide.md). It
contains the current module-boundary map, generated-interface pointers, shared
Cardano/CBOR review invariants, and a per-repository orientation table.

## Workflow

1. Establish the boundary. Run `git status --short`, identify the repository
   being changed, and determine whether the requested change belongs in the
   parent workspace or inside a submodule.
2. Read local guidance before editing: `AGENTS.md`, `CONTRIBUTING.md`,
   `CODEOWNERS`, `README.md`, and relevant package or deployment documentation.
   Local instructions refine this skill.
   For Go work, also inspect every nested `go.mod` under the target repository;
   a root test does not automatically cover those modules.
3. Classify the project using
   [repository-families.md](references/repository-families.md). Select checks
   from the project itself; do not infer that a similar name means identical
   build behavior.
4. Inspect `.github/workflows/` and the corresponding entry in
   `repos/actions/repos-config.yaml`. Generated wrapper workflows are outputs
   of the governance engine; change the source configuration or the shared
   reusable workflow when that is the requested scope.
5. Run the narrowest meaningful checks first, then broader checks when time and
   dependencies permit. Record skipped checks and the reason.
6. Preserve unrelated changes, generated files, credentials, local agent
   state, and untracked worktrees. Do not commit changes inside a submodule from
   the parent repository.
7. Keep plans and planning files local and ephemeral; never commit them. Use a
   repository issue for durable work tracking, acceptance criteria, and follow-up.

## Validation by project family

- Go projects: prefer the repository's `Makefile` targets. Otherwise use
  `gofmt`, `go test ./...`, `go vet ./...`, `golangci-lint`, and NilAway as
  applicable. Check nested modules such as `openapi/` separately when present.
- Docker projects: inspect the Dockerfile and image/tag conventions, prefer a
  matching `blinklabs-io` image whenever one is available, and document any
  fallback to upstream or third-party images. Run `docker build --check .` when
  available and run `actionlint` against changed workflows. Full multi-stage or
  Cardano/Haskell builds can be expensive; do not push images or manifests
  without explicit authorization.
- Mobile, package, and compose projects: read their package/build files and
  local README before selecting commands. Use the workflow inputs in
  `repos/actions` as a clue, not a substitute for project documentation.
- Public docs site: use the package manager named by its README (currently
  `pnpm`), prefer `pnpm install --frozen-lockfile` and `pnpm build`, and update
  navigation metadata such as `pages/_meta.json` when adding pages.
- Knowledge base: treat Markdown structure as the build contract. Preserve
  each book's README, `00-start-here.md`, glossary, source map, and pinned
  public GitHub source links; use a link checker when one is available.
- Protocol/library repositories: use the repository's `Makefile` and Go tests;
  run `buf lint`/`buf generate` for protobuf repositories, and preserve fuzz or
  benchmark coverage where the project defines it.
- Dingo: also apply the dedicated
  [dingo-maintainer](../dingo-maintainer/SKILL.md) skill. Isolate work in a
  worktree, preserve local agent state, avoid `time.Sleep()` in synchronization
  tests, reuse `ouroboros-mock` fixtures, and run race, architecture-boundary,
  conformance, and devnet checks for consensus or protocol changes. Treat
  `DATABASE.md` and `ARCHITECTURE.md` as required change-bar documents.
- Infrastructure repositories: use `ansible-test`/`ansible-lint` for Ansible,
  `helm lint`/`helm template` for charts, and `terraform fmt -check` plus
  `terraform validate` per Terraform module. Do not substitute a generic Go or
  Docker check for the project's native validation.
- Workspace or submodule changes: run `git diff --check`, review
  `git diff --submodule=log`, and verify `git submodule status --recursive`.

## Governance and commits

- Follow the organization guidance in `repos/.github/CONTRIBUTING.md` and
  `repos/.github/SECURITY.md`.
- Use Conventional Commits. Use `git commit -s` for the DCO sign-off.
- Keep a submodule pointer update separate from source changes made in the
  nested repository unless the user explicitly requests a different workflow.
- Treat `repos/actions` as shared infrastructure: it supplies reusable
  workflows and a governance engine that can write generated wrappers and
  repository settings directly to managed repositories' default branches.
- Use canonical upstream repositories and Go modules for source dependencies.
  Blink Labs forks are emergency-only exceptions requiring explicit approval,
  issue tracking, and an exit plan; Apollo is upstream-only under normal
  circumstances.

## Historical session context

When local Codex session records are available, search them by repository path
and name for prior investigations, validation commands, and recurring failure
modes. Use them as historical context only: verify every conclusion against the
current checkout, and never copy credentials, untracked files, or stale fixes.
Distill repeated, repository-independent procedures into this skill or its
references; leave one-off bug details in the project repository.
