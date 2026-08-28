---
name: tosidrop-repo-maintainer
description: Maintain and review TosiDrop repositories, including web, vm-sdk, vm-frontend, infrastructure, docs, and other TosiDrop-owned projects, using their repository, VM API, Cloudflare, npm release, validation, and GitHub rules. Use for any TosiDrop/* task; do not import Blink Labs governance unless the target repository explicitly adopts it.
---

# TosiDrop Repository Maintainer

TosiDrop is a separately governed organization under the same ownership as the
Blink Labs workspace. Reuse shared technical disciplines where they fit, but
do not treat TosiDrop as another `blinklabs-io` repository.

Read [references/repository-map.md](references/repository-map.md) for the active
repository map, contract boundaries, and validation profiles. Verify the live
inventory and target repository before relying on the map.

## Governance boundary

1. Identify the repository from its Git remote or GitHub owner. TosiDrop
   repositories are standalone checkouts unless the current workspace actually
   records one as a submodule; do not invent a `clanker` pointer update.
2. Read the target's `AGENTS.md`, `CLAUDE.md`, contribution and security files,
   README, package manifests, workflows, and `CODEOWNERS`. Live branch
   protection and merge settings are part of the target's rules.
3. Do not inherit Blink-only requirements by proximity to this toolkit. DCO
   sign-off, Conventional Commits, mandatory UI screenshots, the Blink bot-first
   sequence, `blinklabs-io` image preference, author-only merges, and Blink
   release rules apply only when TosiDrop's target repository or the user says
   they do.
4. Preserve published history. Do not rebase, amend, or force-push an existing
   remote branch without explicit authorization for that branch.
5. Keep secrets and operational data private. Never quote credentials,
   Terraform state, plan values, or deployment-variable values in logs,
   reviews, issues, or handoffs. Use private reporting for a vulnerability;
   do not open a public issue containing exploit or credential detail.

## Technical workflow

1. Map the request to the owning repository and its consumers. For `web`, trace
   browser code through Pages Functions, `vm-sdk`, the VM API, and deployment
   bindings in `TosiDrop/infrastructure` before changing a response or error
   contract.
2. Distinguish Cardano state from historical application data. Reward and
   promise breakdown rows describe provenance; current delegation comes from
   authoritative account or ledger state. Fee and whitelist displays must not
   silently substitute one for the other.
3. Treat network selection as part of every API contract. Check mainnet versus
   preview configuration, VM base URL, cache keys, CORS, wallet network, and
   Cloudflare production versus preview bindings together.
4. Degrade browser and dashboard reads per record where safe, but surface
   request failures distinctly from valid empty states. Writes, migrations,
   claims, and state advancement fail rather than committing partial results.
5. Use [cardano-app-reviewer](../cardano-app-reviewer/SKILL.md) for wallet,
   claim, transaction, delegation, and VM behavior; use
   [cross-boundary-changes](../cross-boundary-changes/SKILL.md) for API or
   persisted-shape changes; and use
   [infrastructure-reviewer](../infrastructure-reviewer/SKILL.md) for Terraform
   or Cloudflare deployment review. The Blink live-infrastructure operator does
   not authorize or govern TosiDrop operations.

## Pull requests

- Anchor the diff, checks, approvals, and bot results to the current head SHA.
  Read review submissions, inline threads, and ordinary PR comments.
- Read a bot's result rather than its green status. In TosiDrop repositories,
  CodeRabbit may report success while explicitly saying manual review is
  required; that is a skip, not a clean review.
- Follow the target's current approval and merge settings. Do not assume that a
  policy configured on `web` also applies to `docs`, `plsk`, a private
  repository, or an archived project.
- A review request authorizes inspection and a report, not posting a GitHub
  review. Mutate GitHub only when the user asks for that write.

## Releases and operations

- `vm-sdk` publishes from an exact `vX.Y.Z` tag whose version matches
  `package.json`. The workflow owns `npm publish`; never publish the package or
  create a GitHub release manually. Tagging still requires explicit release
  scope from the user.
- TosiDrop infrastructure plans need the configured remote state and provider
  credentials. Apply, destroy, workflow dispatch, D1 migration, secret update,
  and deployment mutation each require explicit authorization. Read plans for
  sensitive output before sharing them.

Run the target's native checks and finish with
[evidence-based-handoff](../evidence-based-handoff/SKILL.md): exact commands,
directories, exit codes, skipped checks, current head SHA for reviews, and
findings in severity order.
