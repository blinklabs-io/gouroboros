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

Run these rather than listing them as owed. Neither is as expensive as it sounds:

- `go test ./internal/test/conformance/` needs **no setup**. It defaults to an
  in-memory SQLite backend and completes in seconds; PostgreSQL and MySQL are
  opt-in backends for the same vectors, not prerequisites. There is no excuse for
  deferring this one.
- `internal/test/devnet/run-tests.sh` needs a Docker daemon and builds images on
  first run. `--accelerated` runs the bounded event-driven timeline instead of
  the canonical-timing full suite, which is the right choice for a review pass;
  add `--conformance` to bring up the dingo + `cardano-node` reference topology
  when wire or era compatibility is in question.

Check whether the devnet actually exercises the code under review. Its compose
environment does not enable new opt-in flags on its own, so a feature behind a
default-off flag runs switched off and a green devnet says nothing about it —
grep `internal/test/devnet/` for the flag before treating the run as coverage.

Antithesis workflows are dispatch-oriented and should remain isolated from
ordinary local CI. Do not claim live devnet, conformance, registry, or
Antithesis validation unless it was actually run.

For long-running or background checks, inspect the complete output and exit
code after the task finishes and verify that the intended test or gate was
actually exercised. A task notification is not a test result: a completion
notice can report success for a run whose log ends in `FAIL` with a non-zero
exit.

Size the timeout before reading a timeout as a failure. `ledger` runs roughly
9-13 minutes under `-race`, past `go test`'s 10-minute default, and CI uses
`-timeout 20m`. A default-timeout kill reports `panic: test timed out` and names
whichever test was running when the alarm fired, which is not the cause. Use unique ports,
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

## Component lifecycle conventions

Two patterns recur across Dingo's components, and both have bitten recently.

**A component's `Stop` returns an unprefixed error; the caller adds the name.**
`node_shutdown.go` and `node_lifecycle.go` wrap roughly thirty components as
`fmt.Errorf("<component> shutdown: %w", stopErr)`. A `Stop` that also prefixes
itself produces `peer governor shutdown: peer governor shutdown: …` in the
joined error. When a review flags the doubled prefix, fix the component rather
than the call site, so it matches its siblings.

**A `Stop` that takes a context must bound every wait inside it.** An
unbounded `UnsubscribeAndWait`, a `wg.Wait()`, or a listener close placed before
the context-aware wait lets one stuck handler overrun the shutdown deadline.
Use the context-aware variant (`EventBus.UnsubscribeAndWaitContext`), and keep
the unsubscribe itself unconditional so future deliveries stop even when the
wait is cut short.

Related, and still open as issue #3217: the API servers call `net.Listen` and
hand the listener to `Serve` in a goroutine nobody waits for, while `Stop` calls
only `srv.Shutdown` — which closes only listeners `Serve` has already
registered. Stopping inside that window returns with the port still bound, and
`reinitializeAPIServers` then fails to rebind on a live Restore/Truncate. Fixed
in `api/mesh`; `api/blockfrost` and `api/utxorpc` still have it. The shape that
works: record the listener, detach it with the server under one lock, close it
after `Shutdown` (tolerating `net.ErrClosed`), publish it only while the
call's server is still current, and make `Stop` wait for an in-flight bind and
for a teardown another caller won.

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
