# Agent toolkit catalog

Everything the Blink Labs Agent Toolkit provides, and when it applies. The
canonical source is `plugins/blink-labs-agent-toolkit/`; `skills/` is a symlink
into it.

Claude Code loads skills automatically when a task matches a description, and
namespaces the explicit forms as `/blink-labs-agent-toolkit:<name>`. Codex reads
the same `SKILL.md` files.

## Slash commands

| Command | Use it to |
|---|---|
| `/orient` | Locate the owning repository, classify its family, read local instructions, and load the right skills before editing |
| `/validate` | Run the narrowest meaningful checks for a change and produce an evidence ledger |
| `/review` | Run the review loop: domain findings, bot reconciliation, human review requested last |
| `/dep-audit` | Audit Go module graphs, nested modules, replacements, and fork provenance |
| `/release-check` | Review a Docker image or release workflow from Dockerfile through manifest and tags |
| `/submodule-sync` | Inspect and record submodule pointer changes without crossing repository boundaries |

## Subagents

| Subagent | Scope |
|---|---|
| `blink-repo-scout` | Read-only: find which repository owns a symbol, fixture, image, or workflow, and who consumes it |
| `blink-protocol-auditor` | Ledger, CBOR, Ouroboros, consensus, Plutus, conformance fixtures |
| `blink-app-auditor` | Wallet and key safety, transaction construction, DEX and indexer correctness, node integration |
| `blink-go-module-auditor` | Module graph, nested modules, replacements, module identity, provenance |
| `blink-release-auditor` | Dockerfiles, architecture coverage, manifests, tags, publishing workflows |
| `blink-validation-runner` | Executes repository-native checks in isolation and reports commands and exit codes |

## Skills

### Orientation and workspace

| Skill | Use it when |
|---|---|
| `blink-workspace-navigator` | You need to find the repository that owns a topic, or decide where a change belongs |
| `blink-repo-maintainer` | You are making a repository-aware change and need the family, checks, and governance rules |

### Cardano and Go engineering

| Skill | Use it when |
|---|---|
| `cardano-protocol-reviewer` | Ledger eras, CBOR and wire format, Ouroboros mini-protocols, consensus, Plutus, conformance |
| `cardano-app-reviewer` | Wallets, keys, transactions, DEX and indexer behavior, node integration, provider responses |
| `dingo-maintainer` | Any change to the Dingo node: architecture boundaries, events, storage, devnet and conformance |
| `go-api-maintainer` | OpenAPI, protobuf and Buf, ConnectRPC, sqlc, nested modules, public Go compatibility |
| `go-dependency-auditor` | `go.mod`/`go.sum` changes, missing checkouts, replacements, upstream-versus-fork provenance |

### Delivery and operations

| Skill | Use it when |
|---|---|
| `dingo-block-producer-operator` | Operating or troubleshooting an existing Dingo block producer: lifecycle, forging health, KES/opcert rotation, HA, and incident evidence |
| `docker-release-reviewer` | Docker images, multi-architecture builds, manifests, registry publication |
| `infrastructure-reviewer` | Helm, Terraform, Ansible, Kubernetes operators, helmfile, compose stacks |
| `docs-kb-maintainer` | Public documentation site content and knowledge-base structure |

### Process discipline

| Skill | Use it when |
|---|---|
| `cross-boundary-changes` | Changing a shape something else depends on — response bodies, error paths, public fields, ID formats, persisted keys |
| `regression-test-discipline` | Adding a test alongside a fix, or a test asserts something the test itself performed |
| `isolated-validation-runs` | A check is slow, stateful, networked, or concurrent — worktrees, unique resources, flake triage |
| `evidence-based-handoff` | Finishing work: evidence ledger, skipped-check list, findings in severity order |
| `commit-and-pr-hygiene` | Writing a commit, PR description, or review comment |
| `github-review-coordinator` | Discovering direct and team review requests, sequencing bot review, fix loops, reviewer re-requests, and dismissal |
| `agent-toolkit-authoring` | Changing the toolkit itself: skills, commands, subagents, hooks, manifests |

## Workspace guards (hooks)

| Hook | Event | Behavior |
|---|---|---|
| `git-commit-guard.py` | `PreToolUse` on `Bash` | Denies a `git commit` missing DCO sign-off or a Conventional Commit subject; warns when a plan file is staged. Bypass with `BLINK_SKIP_COMMIT_GUARD=1` |
| `submodule-boundary-notice.py` | `PreToolUse` on edits | Emits one notice per submodule per session when editing under `repos/`. Never blocks. Disable with `BLINK_SKIP_BOUNDARY_NOTICE=1` |
| `workspace-brief.py` | `SessionStart` | Reports uninitialized submodules, already-moved pointers, and dirty submodules. Disable with `BLINK_SKIP_WORKSPACE_BRIEF=1` |

All guards fail open: unparseable input exits without blocking.

## Shared references

| Reference | Contents |
|---|---|
| `docs/go-repository-guide.md` | Go common ground: module boundaries, generated interfaces, CBOR and fixture invariants, per-repository pointers |
| `docs/repository-patterns.md` | Workspace architecture, project families, validation matrix, governance implications |
| `skills/blink-repo-maintainer/references/repository-families.md` | Per-family workflow profiles and validation |
| `skills/blink-workspace-navigator/references/ownership-map.md` | Topic-to-repository map, dependency spine, disambiguation traps |
| `skills/dingo-maintainer/references/dingo-agent-workflow.md` | Dingo investigation, live-run, and review checklist |
| `skills/infrastructure-reviewer/references/deployment-checklist.md` | Terraform, Helm, Ansible, Kubernetes, and compose review checks |

## Validating a toolkit change

```sh
make validate
```

Checks manifest JSON, skill front matter and `name`-to-directory match, command
and subagent metadata, hook syntax and behavior, symlink integrity, and internal
link resolution.
