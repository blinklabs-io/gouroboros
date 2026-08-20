# Go repository common ground

This is the starting point for agents reviewing or changing a Blink Labs Go
repository from the `clanker` workspace. It records patterns found across the
current checkouts and points to the project-owned source of truth. It does not
replace a repository's `AGENTS.md`, `CLAUDE.md`, `CONTRIBUTING.md`, README, or
Makefile; local guidance wins when it is more specific.

## Start here

1. From the workspace root, run `git status --short` and
   `git submodule status --recursive`. Confirm whether the change belongs in
   the parent or in a submodule.
2. Read the root [`AGENTS.md`](../AGENTS.md), then the target repository's
   `AGENTS.md`, `CLAUDE.md`, `CONTRIBUTING.md`, `README.md`, and `CODEOWNERS`.
3. Read the target module's `go.mod`, `Makefile`, `.golangci.yml`, and
   `.github/workflows/`. A Go repository may contain several independent
   modules, so discover them with:

   ```sh
   rg --files repos/<repository> -g 'go.mod' -g '!**/.git/**'
   ```

4. Check `repos/actions/repos-config.yaml` and the corresponding workflow
   files. The generated wrappers show the CI contract, while `actions` is the
   source of reusable workflow behavior.
5. Identify the affected contract before choosing tests: public Go API,
   Cardano ledger rule, Ouroboros protocol, CBOR encoding, generated API,
   storage/schema, command-line behavior, UI boundary, or documentation.
6. Run the narrowest repository-native check first, then expand to the checks
   required by that contract. Report skipped live, registry, devnet, or
   conformance checks explicitly.

Do not run `go mod tidy` from the workspace root or assume that a root-module
test covers nested modules. A tidy operation can change dependency metadata;
review `go.mod` and `go.sum` as part of the change.

## Review tooling common ground

Codex and Claude should use this guide and the target repository's local
instructions as their shared context. CodeRabbit and Cubic findings should be
treated as hypotheses to verify against the current checkout, not as an
additional source of truth. For every bot finding, record the exact path and
symbol, reproduce or disprove it with a focused check, and classify it as a
blocker, recommendation, false positive, or already addressed.

Do not copy bot-generated prose into a README, knowledge-base page, skill, or
issue without verification. Durable documentation should explain the
repository behavior and point to the owning source file, test, workflow, or
specification.

Commit messages, PR descriptions, and review comments are review evidence.
Keep them short, factual, and scoped to the changed code and checks. Do not
include storytelling, chat transcripts, roadmaps, future plans, or unrelated
context. Put code-specific feedback in inline comments; use PR-level text only
for concise summaries, validation, or disposition.

The review sequence is bot review first, human review second. Run CodeRabbit
and Cubic when configured, address their actionable findings, and only then
request human review. If CodeRabbit is rate-limited, document it; a completed
Cubic review is sufficient for bot review. Human review is mandatory and may be
AI-assisted; bot approval or silence is not human approval.

Review scope is explicit: do not review draft PRs. If Dependabot is excluded,
filter `dependabot[bot]` before inspecting diffs. Verify the current head SHA,
requested-reviewer state, checks, and latest bot findings; older human reviews
must be re-evaluated after new commits.

If a human reviewer requests changes, implement and validate the fixes, record
the response and changed paths on the pull request, and use GitHub's review
request action to request another review from that same reviewer. A pushed fix
or review-thread reply does not replace the explicit follow-up review.

UI changes require screenshots in the pull request. Include the affected states
at the relevant viewport or platform, with secrets and user data redacted.

An authorized human reviewer may dismiss another human review through GitHub
when appropriate. Record the rationale on the pull request; dismissal does not
eliminate the requirement for appropriate human review.

Only the author merges their own pull request — whoever merges takes
responsibility for the code — with `dependabot[bot]` the sole exception.

Squash merge is allowed only when GitHub shows human approval for the current
head SHA, required checks pass, and configured bots have no actionable findings.
Use one concise factual squash summary and preserve the DCO
`Signed-off-by:` line; an approval for an earlier head is stale after a push.

An issue-resolution request includes the post-update bot loop: verify CodeRabbit
and Cubic findings against each current head, or document CodeRabbit rate
limiting and use Cubic alone, fix valid findings, validate, and repeat until the
available bots have no actionable findings and the PR is ready for human review.

## Common Go baseline

Across the Blink Go repositories, the usual local targets are:

```sh
make mod-tidy
make format
make test
make golines
```

Most governed repositories also run `golangci-lint` and NilAway in CI. Use
`make lint`, `make nilaway`, or the exact workflow command when the project
defines one. Prefer `go test -v -race -run TestName ./path/to/package` for a
focused test, then the repository's full test target. `go vet` remains useful
for small libraries such as `go-bip39`, even when it is not a named Makefile
target.

The Go floor is not uniform. Current root modules range from Go 1.25.0 through
Go 1.26.0, and generated, example, UI, and OpenAPI modules can have different
directives. Read each module's `go` directive and honor its toolchain floor;
do not lower it to make a local build pass.

### CI toolchain and scanner versions float on purpose

Workflow `go-version` values are floating minors (`1.25.x`, `1.26.x`), and
`golangci-lint-action` is pinned by SHA but takes no `version:` input so it
installs the latest release. This is deliberate: a floating version forces a new
advisory or a stricter check to be fixed when it lands, instead of sitting
unnoticed behind a stale pin.

Two consequences to hold onto:

- **Never pin a version to make a check pass.** A red check under a floating
  version is the system working. Fix the underlying problem, and reject review
  suggestions that propose a pin as the remedy — including bot suggestions that
  name a specific patch release. That pin is stale by the next release, which is
  how the failure arose in the first place.
- **Scanners may run ahead of the toolchain we ship, and that is intended.** We
  want the scan against the newest stdlib and the newest advisory data, so a
  `govulncheck` job on `1.26.x` while `publish.yml` builds `1.25.x` is correct.
  Do not pull a scanner back to match the release toolchain.

What protects a release is that `publish.yml` floats too, so the shipped stdlib
is always the latest patch of its minor. An exact pin in `publish.yml` is the
real defect: cdnsd shipped on a pinned `1.25.12` carrying five advisories that
`1.25.13` had already fixed. Float the publish job; leave the scanner ahead.

Know the residual gap so you can describe it accurately: `govulncheck` reports
standard-library advisories against the toolchain it runs under, so a scanner on
a newer minor will not flag an advisory that is still live on the older minor the
release builds with. A floating `publish.yml` is what closes that, since Go
backports security fixes across supported minors — aligning the scanner
downward would only trade newer coverage for older.

Exact patch pins rot into failures on diffs that have nothing to do with them —
see "A failing check the diff cannot explain" in the
[github-review-coordinator skill](../skills/github-review-coordinator/SKILL.md).

The recurring quality contract is:

- format with the repository's target, including `golines` where configured;
- keep tests deterministic and use race detection for concurrent code;
- add tests for changed exported behavior and malformed or boundary inputs;
- preserve generated-code provenance and regenerate only from its source;
- run NilAway and golangci-lint when the repository's CI requires them;
- use Conventional Commits and DCO sign-off for commits;
- keep Docker image choices aligned with `blinklabs-io` images when available.

## Dependency provenance

Use the canonical upstream repository and Go module for source dependencies.
Blink Labs forks are emergency-only exceptions: they require explicit approval,
an issue recording the reason and exit plan, and a clear handoff for returning
to upstream. Apollo is always upstream under normal circumstances:
`github.com/Salvionied/apollo/v2` from `Salvionied/apollo`.

## Dependency spine

Use the checked-out module graph as the source of truth; this map is an
orientation aid for deciding where a fix or fixture belongs:

| Layer | Repositories and usual role |
| --- | --- |
| Ledger and protocol core | [`gouroboros`](../repos/gouroboros) provides Cardano ledger types, CBOR, crypto, and Ouroboros mini-protocols; [`plutigo`](../repos/plutigo) provides UPLC evaluation; [`cardano-models`](../repos/cardano-models), [`go-bip39`](../repos/go-bip39), [`go-scls`](../repos/go-scls), and [`merkle-patricia-forestry`](../repos/merkle-patricia-forestry) provide focused data or format libraries |
| Shared test surface | [`ouroboros-mock`](../repos/ouroboros-mock) provides network conversations, ledger state, protocol parameters, consensus scenarios, and conformance fixtures for downstream projects |
| Wallet and transactions | [`bursa`](../repos/bursa) provides wallet/key and transaction functionality; external upstream [`Salvionied/apollo`](https://github.com/Salvionied/apollo) provides transaction construction and backend adapters |
| Node and chain services | [`dingo`](../repos/dingo) is the Go Cardano node; [`adder`](../repos/adder) provides chain-sync event processing; [`bark`](../repos/bark) defines Dingo operations APIs; [`cdnsd`](../repos/cdnsd) implements Cardano DNS; [`handshake-node`](../repos/handshake-node) is a security-sensitive Handshake node |
| Applications and APIs | [`shai`](../repos/shai), [`bluefin`](../repos/bluefin), [`cardano-node-api`](../repos/cardano-node-api), [`tx-submit-api`](../repos/tx-submit-api), [`tx-submit-api-mirror`](../repos/tx-submit-api-mirror), [`nview`](../repos/nview), [`txtop`](../repos/txtop), [`dingoctl`](../repos/dingoctl), [`dns-cli`](../repos/dns-cli), and [`vpn-indexer`](../repos/vpn-indexer) consume the protocol and service layers |
| Packaging and governance | [`cardano-up`](../repos/cardano-up) manages packages and contexts; [`actions`](../repos/actions) defines the shared CI/governance behavior |

When a change appears to need a new mock, protocol type, transaction model, or
generated client, trace this spine before adding a local duplicate. The full
declared Blink module set can be inspected with:

```sh
rg -n --glob 'go.mod' --glob '!**/.git/**' 'github.com/blinklabs-io/' repos
```

## Review invariants shared across projects

### Cardano and CBOR

`gouroboros` is the central ledger, CBOR, and Ouroboros protocol dependency.
Read its [agent guide](../repos/gouroboros/AGENTS.md) and the relevant
`ledger/<era>/` or `protocol/<name>/` package before reviewing downstream
adapters. Do not assume a later-era validation rule delegates to an earlier
era; inspect the function body and its tests.

Types embedding `cbor.DecodeStoreCbor` preserve original bytes. Hashes and
wire-compatible re-encodings must use the preserved `.Cbor()` bytes. After
mutating a decoded value, call `SetCbor(nil)` before marshaling if the change
must be encoded. Preserve Cardano map ordering and indefinite/definite CBOR
choices where the protocol requires them.

Public Go serialization methods are API surface. Changing a value receiver to
a pointer receiver can make `json.Marshal(value)` bypass the custom encoder;
trace value call sites and test both value and pointer inputs. Guard typed-nil
values in type switches. Check the module's Go directive before accepting
range-variable alias findings from review bots.

For generated workflow wrappers, verify every `blinklabs-io/actions` workflow
path exists at the referenced source ref. Pin release and secret-bearing calls
to full commit SHAs and declare only the permissions the called workflow needs.

### Shared fixtures and conformance

`ouroboros-mock` is the shared source for network conversations, ledger state,
protocol parameters, consensus scenarios, and conformance vectors. Its
[fixtures README](../repos/ouroboros-mock/fixtures/README.md),
[conformance README](../repos/ouroboros-mock/conformance/README.md), and
[consensus README](../repos/ouroboros-mock/consensus/README.md) are the first
places to look before creating test data. Extend that repository and bump the
dependency instead of copying fixtures into Dingo, Adder, Shai, Apollo, or
another downstream project.

Protocol or ledger changes should normally include deterministic vectors or
conformance coverage. Use fuzzing for parsers, decoders, and evaluators, and
benchmarks for hot paths or performance-sensitive changes. A passing unit test
does not substitute for the applicable conformance or integration suite.

### Generated interfaces

Adder, Bursa, Cardano Node API, and Tx Submit API keep generated OpenAPI clients
or server surfaces in nested `openapi/` modules and expose a root `swagger`
or `openapi.sh` entry point. Review the OpenAPI source and generation command
before editing generated files; test the nested module separately.

`bark` keeps protobuf definitions under `proto/` and generated ConnectRPC/Go
files beside them. Use its Buf configuration and README commands, and treat
`PROTOCOL_DESIGN.md` as the protocol contract.

Dingo has both protobuf and sqlc-generated database code. Use `make proto` and
`make sql` only when their inputs changed, and use `make sql-check` to detect
stale checked-in output.

### Module boundaries and local replacements

Nested modules are deliberate isolation boundaries, not ordinary packages:

- `adder/openapi`
- `bursa/openapi` and `bursa/ui`
- `cardano-node-api/openapi`
- `dingo/examples/dingo-gov-lens` and `dingo/internal/test/antithesis`
- `vpn-indexer/openapi`
- `go-scls/cmd/scls`
- `gouroboros/examples/*`
- `tx-submit-api/openapi`

Check each nested module's own `go.mod`, replacement directives, tests, and
workflow before making a root-module assumption. The examples under
`gouroboros/examples` intentionally replace the checked-out parent module for
local development; `go-scls/cmd/scls` does the same for its library.

Apollo is intentionally not a workspace submodule. It declares the upstream
module path `github.com/Salvionied/apollo/v2`, and Shai is transitioning back to
that upstream dependency from its Blink Labs module path. Until the Shai
repository completes that transition, do not add a local replacement or
rewrite its module metadata from the parent workspace; treat any temporary
build failure as an issue for the Shai dependency update.

## Repository pointers

| Repository | Read first | Review and validation focus |
| --- | --- | --- |
| [`actions`](../repos/actions) | `README.md`, `repos-config.yaml` | Reusable workflow inputs, generated wrapper ownership, and direct writes to downstream default branches |
| [`adder`](../repos/adder) | `README.md`, `Makefile`, `openapi/README.md` | Chainsync/mempool inputs, event filtering, library examples, and generated API clients |
| [Apollo upstream](https://github.com/Salvionied/apollo) | upstream `AGENTS.md`, `CONTRIBUTING.md`, `backend/base.go` | External transaction builder, `ChainContext` backends, deterministic fixed backend tests, and CBOR; not tracked as a submodule |
| [`bark`](../repos/bark) | `README.md`, `PROTOCOL_DESIGN.md` | Proto source, Buf formatting/lint/generation, ConnectRPC compatibility, and Dingo integration |
| [`bluefin`](../repos/bluefin) | `README.md`, `Makefile` | Miner/indexer transaction flow, OpenCL build path, benchmarks, and its Adder/Bursa/model dependencies |
| [`bursa`](../repos/bursa) | `README.md`, `Makefile`, `ui/`, `openapi/` | Wallet/key handling, API generation, nested UI module, mobile workflow, and sensitive seed material |
| [`cardano-models`](../repos/cardano-models) | `README.md`, `Makefile` | Stable model/API compatibility and gouroboros type usage |
| [`cardano-node-api`](../repos/cardano-node-api) | `README.md`, `Makefile`, `openapi/` | Node protocol adapters, generated REST surface, and nested OpenAPI tests |
| [`cardano-up`](../repos/cardano-up) | `README.md`, `packages/` | CLI/package-manager behavior, contexts, package definitions, and version consistency |
| [`dingo`](../repos/dingo) | `AGENTS.md`, `CLAUDE.md`, `ARCHITECTURE.md`, `DATABASE.md`, `Makefile` | Architecture boundaries, EventBus, plugin composition, storage migrations, race tests, conformance, and devnet; use the [Dingo agent workflow](../skills/dingo-maintainer/references/dingo-agent-workflow.md) for live investigations and review findings |
| [`cdnsd`](../repos/cdnsd) | `README.md`, `Makefile`, `handshake/`, `internal/indexer/` | DNS and Handshake validation, recursive DNSSEC behavior, peer safety, and generated/runtime checks |
| [`dingo-operator`](../repos/dingo-operator) | `AGENTS.md`, `CLAUDE.md`, `README.md`, `Makefile` | CRD reconciliation, envtest, non-root containers, Dingo lifecycle, and Helm packaging |
| [`dingoctl`](../repos/dingoctl) | `README.md`, `Makefile` | TLS on every command, mTLS for resource-consuming RPCs, and Bark compatibility |
| [`handshake-node`](../repos/handshake-node) | `README.md`, `go.mod`, `hnsutil/` | P2P/RPC security, consensus-sensitive validation, nested modules, and fork/upstream boundaries |
| [`docker-wireguard`](../repos/docker-wireguard) | `README.md`, `Makefile` | API/security behavior, Docker image checks, JWT handling, and NilAway |
| [`go-bip39`](../repos/go-bip39) | `AGENTS.md`, `README.md`, `Makefile` | BIP-39 vectors, wordlists, cryptographic compatibility, and seed-handling security |
| [`go-scls`](../repos/go-scls) | `AGENTS.md`, `CLAUDE.md`, `spec/RECONCILIATION.md` | Wire-format invariants, cross-implementation vectors, fuzzing, conformance, and nested CLI module |
| [`gouroboros`](../repos/gouroboros) | `AGENTS.md`, `CLAUDE.md`, `ledger/`, `protocol/` | Era rules, mini-protocol state machines, CBOR, crypto, examples, fuzzing, and conformance |
| [`merkle-patricia-forestry`](../repos/merkle-patricia-forestry) | `README.md`, `Makefile` | Hash/tree semantics, deterministic tests, and its Bluefin/gouroboros dependency role |
| [`nview`](../repos/nview) | `README.md`, `prometheus.go` | Metrics parsing, Dingo/Amaru implementation detection, TUI behavior, and remote monitoring |
| [`ouroboros-mock`](../repos/ouroboros-mock) | `README.md`, `fixtures/`, `conformance/`, `consensus/` | Shared test data, vector provenance, conversation harnesses, and consensus scenarios |
| [`plutigo`](../repos/plutigo) | `AGENTS.md`, `README.md`, `Makefile`, `DEVELOPMENT.md` | CEK evaluation, builtin availability/cost models, conformance, replay, fuzzing, and benchmarks |
| [`shai`](../repos/shai) | `AGENTS.md`, `README.md`, `internal/config/profiles.go` | Indexer/node/DEX data flow, profile parameters, transaction construction, and Apollo/Dingo integration |
| [`tx-submit-api`](../repos/tx-submit-api) | `README.md`, `Makefile`, `openapi/` | Submission semantics, node connection, generated API, and Docker runtime |
| [`tx-submit-api-mirror`](../repos/tx-submit-api-mirror) | `README.md`, `Makefile` | Mirror/failover behavior, submission semantics, generated/runtime parity, and Docker image |
| [`txtop`](../repos/txtop) | `README.md`, `Makefile`, `main.go` | Mempool inspection, Cardano metrics/model compatibility, and CLI output stability |
| [`vpn-indexer`](../repos/vpn-indexer) | `README.md`, `Makefile`, `openapi/` | Go API/indexer behavior, generated OpenAPI module, and service configuration |

## Review report baseline

When reviewing a Go change from this workspace, report findings in this order:

1. Behavioral or security defects, especially protocol, ledger, key, storage,
   or concurrency errors.
2. Public API and wire-format compatibility, including generated interfaces.
3. Missing deterministic, race, conformance, fuzz, benchmark, or integration
   coverage for the changed contract.
4. Architecture or dependency-boundary violations.
5. Documentation and generated-artifact drift.
6. Style, lint, and follow-up improvements.

For every skipped check, state why it was skipped. Distinguish a merge blocker
from a non-blocking recommendation, and include the exact repository path and
test or workflow that supports the conclusion.
