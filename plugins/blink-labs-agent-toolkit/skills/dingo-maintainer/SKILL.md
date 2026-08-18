---
name: dingo-maintainer
description: Maintain Blink Labs' Dingo Cardano node safely using its architecture boundaries, shared Ouroboros fixtures, race-enabled tests, conformance and devnet validation, documentation requirements, and repository-specific lint checks. Use when changing Dingo source, tests, storage, networking, consensus, plugins, integration environments, or architecture documentation.
---

# Dingo Maintainer

Use this skill for changes in `repos/dingo` or for changes in shared projects
that affect Dingo behavior. Dingo is a Go implementation of a Cardano node;
small changes can cross ledger, storage, networking, protocol, and composition
boundaries, so validate the affected contract rather than relying only on a
package-local test.

## Before editing

1. Read `repos/dingo/AGENTS.md`, `CLAUDE.md`, `README.md`, and the relevant
   architecture or database documentation.
2. Inspect existing issues and pull requests for the requested behavior before
   designing a duplicate fix or test.
3. Use an isolated worktree or branch for implementation and review work.
   Preserve the main checkout and any pre-existing `.claude/`, `CLAUDE.md`,
   roadmap, or untracked agent state.
4. Identify whether the change belongs in Dingo or in a shared dependency. Put
   reusable protocol fixtures in `ouroboros-mock`; do not duplicate them in
   Dingo.
5. For live incidents, long validation runs, or review findings, read
   [the Dingo agent workflow](references/dingo-agent-workflow.md). Preserve
   evidence, isolate worktrees and resources, and check current `origin/main`
   before editing.

## Architecture boundaries

- Composition belongs in `node.go`, root-package helpers, `cmd/dingo`, or
  `internal/node`. Do not use package initialization or process-global plugin
  registration.
- Use `event.EventBus` for asynchronous cross-component notifications. Use
  narrow injected interfaces or callbacks for synchronous reads.
- Keep storage and `database/` independent of ledger, mempool, networking,
  node, and API packages. Keep connection management independent of
  Ouroboros and ledger semantics.
- Ledger owns validation, rollback repair, nonce/epoch logic, and ledger
  queries. It should emit neutral events rather than directly controlling
  networking or peer governance.
- Preserve plugin provider boundaries and the documented selector precedence:
  CLI selector, generic plugin environment, YAML, then provider defaults.
- Remember that decoded types embedding `DecodeStoreCbor` can re-emit their
  original bytes. Clear the stored CBOR before marshaling a mutated value.
- Treat `chain.update`, `chain.fork_detected`,
  `chainselection.chain_switch`, `epoch.transition`, mempool events, and
  connection/peer-governance events according to the event table in
  `repos/dingo/AGENTS.md`.

## Validation workflow

Start narrow, then expand according to the affected contract:

```sh
make test
go test -v -race -run TestName ./path/to/pkg/
golangci-lint run ./...
nilaway ./...
modernize ./...
make import-boundaries
make docs-parity
make golines
```

Run `make sql-check` when SQL or `sqlc.yaml` is affected, and run
`make gorm-check` for database changes. Do not interpret a root lint result
without checking whether the failure is pre-existing or in generated code.

Never use `time.Sleep()` to synchronize tests. Use
`internal/test/testutil/WaitForCondition`, `RequireReceive`, or a context with
a timeout.

For changes touching consensus, block production, header/VRF/KES/OpCert
verification, chain selection, mempool, transaction submission, NtN/NtC
protocols, epoch boundaries, or nonce computation, run the Dingo devnet suite
from `internal/test/devnet/run-tests.sh`. Run its `--conformance` mode when
compatibility with `cardano-node` matters, and run the conformance tests in
`internal/test/conformance/` after every such change. Use the real-block
integration fixtures under `internal/integration/` and
`database/immutable/testdata/` where applicable.

Antithesis workflows are dispatch-oriented and should remain isolated from
ordinary local CI. Do not claim live devnet, conformance, registry, or
Antithesis validation unless it was actually run.

For long-running or background checks, inspect the complete output and exit
code after the task finishes and verify that the intended test or gate was
actually exercised. A task notification is not a test result. Use unique ports,
temporary paths, and `GOCACHE` values; keep sync/from-genesis runs serialized,
and compare suspicious failures with an `origin/main` baseline. Do not stop or
reconfigure a live node or validation run while diagnosing it unless the task
explicitly authorizes that intervention.

When reviewing a finding, verify the current control flow and add a focused
test for the reported behavior and its negative case. In particular, check
incumbent eligibility, equal-height selection, lifecycle cancellation,
mutually exclusive flags, persisted-gate absence, script argument/port
handling, and concurrent test state. File an issue for confirmed flakes or
dropped events instead of filtering them away.

For cache or decoder changes, verify the size bound, TTL behavior on hot reads,
in-flight waiter cleanup, panic/error cleanup, direct waiter result delivery,
and retained-object memory cost. Concurrent benchmarks must not call
`Fatalf`/`FailNow` from `RunParallel` worker goroutines.

For Koios/account or configuration changes, verify programmatic defaults and
composition wiring, CLI/YAML/environment precedence, cancellation after the
first failed chunk, duplicate-row preservation, malformed and negative amount
handling, and propagation of non-`sql.ErrNoRows` database errors.

## Documentation and delivery

Treat `DATABASE.md` and `ARCHITECTURE.md` as part of the change bar. Update
`DATABASE.md` for schema, query/API, blob layout, encoding, storage-provider,
pruning, or tombstone changes. Update `ARCHITECTURE.md` for component
responsibilities, package boundaries, startup/composition, EventBus topics,
plugin interfaces, lifecycle/concurrency, or cross-component flows.

In the final report, list those documents if updated or explicitly say they
were checked and unaffected. Follow the organization contribution guidance,
use Conventional Commits, and create commits with `git commit -s`.

Dingo's README currently limits operational claims to testnet, preview, and
devnet contexts; do not describe it as mainnet-ready without an explicit
project decision.

For the full investigation and review checklist, use
`references/dingo-agent-workflow.md`.
