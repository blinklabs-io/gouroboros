---
name: blink-repo-maintainer
description: Inspect, classify, and validate Blink Labs repositories and submodules using organization conventions, generated GitHub Actions workflows, Docker/Go/Cardano project patterns, Conventional Commits, and DCO. Use when working in a Blink Labs repository, adding or updating a repository in the clanker workspace, reviewing a project change, or deciding which checks and shared workflow profile apply.
---

# Blink Repository Maintainer

Use this skill to make repository-aware changes across the Blink Labs project
family. Treat the parent `clanker` repository as a workspace and each entry
under `repos/` as an independently versioned project.

For Go repositories or cross-repository code reviews, read the shared
[Go repository common-ground guide](../../references/go-repository-guide.md). It
contains the current module-boundary map, generated-interface pointers, shared
Cardano/CBOR review invariants, and a per-repository orientation table.

Use the focused skills when applicable:

- [blink-workspace-navigator](../blink-workspace-navigator/SKILL.md) to find
  which repository owns a topic, symbol, fixture, or workflow;
- [cardano-protocol-reviewer](../cardano-protocol-reviewer/SKILL.md) for
  ledger, CBOR, Plutus, Ouroboros, consensus, and conformance work;
- [cardano-app-reviewer](../cardano-app-reviewer/SKILL.md) for wallet,
  transaction, DEX, indexer, and node-integrated applications;
- [dingo-maintainer](../dingo-maintainer/SKILL.md) for the Dingo node;
- [go-api-maintainer](../go-api-maintainer/SKILL.md) for OpenAPI, protobuf,
  ConnectRPC, sqlc, and generated Go surfaces;
- [go-dependency-auditor](../go-dependency-auditor/SKILL.md) for module graph,
  replacement, checkout, and provenance audits;
- [docker-release-reviewer](../docker-release-reviewer/SKILL.md) for image,
  multi-architecture, manifest, and publishing workflows;
- [infrastructure-reviewer](../infrastructure-reviewer/SKILL.md) for Helm,
  Terraform, Ansible, operator, and compose deployment changes;
- [docs-kb-maintainer](../docs-kb-maintainer/SKILL.md) for public docs and KB
  boundaries;
- [github-review-coordinator](../github-review-coordinator/SKILL.md) for the
  bot-first, human-required pull-request review loop;
- [commit-and-pr-hygiene](../commit-and-pr-hygiene/SKILL.md) for Conventional
  Commits, DCO sign-off, and message and description scope;
- [isolated-validation-runs](../isolated-validation-runs/SKILL.md) for
  worktrees, unique resources, background tasks, and flake triage;
- [evidence-based-handoff](../evidence-based-handoff/SKILL.md) for the final
  evidence ledger, skipped-check list, and findings order; and
- [agent-toolkit-authoring](../agent-toolkit-authoring/SKILL.md) when changing
  the toolkit's own skills, commands, subagents, or hooks.

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
- Public docs site: use the active `repos/docs-site` checkout of
  `blinklabs-io/docs`, which uses Astro/Starlight and npm. Prefer `npm ci`,
  `npm run check`, and `npm run build`; add content under `src/content/docs/`
  and preserve Starlight navigation metadata. The archived
  `blinklabs-io/docs-site` repository is not the workspace docs site.
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
  Docker check for the project's native validation. Use
  [infrastructure-reviewer](../infrastructure-reviewer/SKILL.md) for blast
  radius, state safety, and secret handling.
- Workspace or submodule changes: run `git diff --check`, review
  `git diff --submodule=log`, and verify `git submodule status --recursive`.

## Governance and commits

- Follow the organization guidance in `repos/.github/CONTRIBUTING.md` and
  `repos/.github/SECURITY.md`.
- Use Conventional Commits. Use `git commit -s` for the DCO sign-off.
- Keep commit subjects short and factual. If a body is needed, use only short
  factual lines directly tied to the changed code, tests, or review; omit
  storytelling, chat transcripts, roadmaps, future plans, and unrelated
  context.
- Keep PR descriptions and review comments concise, factual, and scoped to the
  current change. Put code-specific feedback in inline comments; reserve
  PR-level comments for concise summaries, checks, or dispositions.
- Keep a submodule pointer update separate from source changes made in the
  nested repository unless the user explicitly requests a different workflow.
- Treat `repos/actions` as shared infrastructure: it supplies reusable
  workflows and a governance engine that can write generated wrappers and
  repository settings directly to managed repositories' default branches.
- Use canonical upstream repositories and Go modules for source dependencies.
  Blink Labs forks are emergency-only exceptions requiring explicit approval,
  issue tracking, and an exit plan; Apollo is upstream-only under normal
  circumstances.
- Follow the review order: run configured CodeRabbit/Cubic checks, address
  actionable findings, then obtain required human review. Human review may be
  AI-assisted, but bot approval or silence is never sufficient by itself.
- Do not review draft pull requests. If Dependabot is excluded, omit PRs
  authored by `dependabot[bot]` before inspecting changes.
- UI changes require screenshots in the PR. Verify the affected states and
  relevant viewport or platform are represented, with secrets and user data
  redacted.
- For generated workflow changes, verify every called workflow exists in the
  current `repos/actions` source at the referenced ref. Pin release and
  security-sensitive reusable workflows to full commit SHAs and keep token
  permissions explicit and minimal.
- If a human reviewer requests changes, validate and push the fixes, summarize
  them on the pull request, and explicitly request another review from that
  same reviewer through GitHub.
- An authorized human reviewer may dismiss another human review through
  GitHub when appropriate, with the rationale recorded on the pull request.

## Historical session context

Local Codex and Claude session records can be searched by repository path and
name for prior investigations, validation commands, and recurring failure
modes. Use them as history only: verify every conclusion against the current
checkout, and never copy credentials, untracked files, or stale fixes. Distill
repeated, repository-independent procedures into this skill or its references;
leave one-off bug details in the project repository. See
[blink-workspace-navigator](../blink-workspace-navigator/SKILL.md) for locating
the owning repository first.
