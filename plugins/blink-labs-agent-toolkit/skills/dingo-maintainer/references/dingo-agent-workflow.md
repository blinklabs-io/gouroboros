# Dingo agent workflow

Use this reference with the Dingo maintainer skill when investigating a live
failure, a large validation run, or a review finding. It distills recurring
patterns from Dingo's repository guidance and agent sessions; the Dingo
checkout remains the source of truth for commands and behavior.

## Start from a known state

- Check current `origin/main`, open issues, and open pull requests before
  proposing a fix. Reuse an existing issue or PR when the work overlaps.
- Put a gouroboros or plutigo defect in the owning library, release it there,
  then adopt it in a separate dependency-only Dingo pull request. Do not ship a
  Dingo-side compatibility workaround for an upstream protocol defect or mix
  the dependency bump with unrelated Dingo code.
- Start implementation or review work in a fresh worktree based on the
  intended remote base. Keep the main checkout, other worktrees, and existing
  `.claude/` or other agent state intact.
- Recheck `origin/main`, the tracker, and overlapping pull requests after long
  validation and immediately before publication. Unrelated main movement does
  not invalidate a focused result, but a dependency change that touches the
  same `go.mod` or `go.sum` requires a fresh graph selection, tidy, and focused
  compatibility run on the new base.
- Split unrelated fixes into separate, focused branches and PRs. Do not rewrite
  history to make the result look cleaner.
- If a live node, devnet, or validation run is in progress, treat it as an
  observation target. Do not stop, restart, reconfigure, or delete its data
  unless the task explicitly authorizes that intervention.

## Coordinate a shared audit ledger

When disclosure and the task scope authorize tracker writes, treat the audit
report as shared mutable state rather than a static input.

- Resolve the user-named canonical report at the start and again before
  handoff. Another agent may create a consolidated report, rename the active
  ledger, or revise severity while implementation is in progress.
- If the workflow requires issue-before-work, make the successful issue write
  the implementation gate. Assign the requested owner and mark the exact
  finding row or section before editing source. Add the PR and head after they
  exist.
- Patch only the findings you own and re-read their surrounding text before
  every update. Do not overwrite another agent's disposition, and do not commit
  an otherwise local audit report unless the task explicitly asks for it.
- Before handoff, reconcile the canonical report with live GitHub state:
  issue assignment, PR head, required checks, bot review that actually ran,
  unresolved findings, and human approval. If a later consolidation changes
  severity, preserve the tracker and state the reclassification rather than
  erasing the earlier work.

## Run validation reproducibly

- Treat required checks as required. If a check cannot run, record the exact
  command and reason; do not silently skip it.
- Use unique ports, temporary directories, profiling databases, logs, and
  `GOCACHE` values for concurrent runs. Fixed `/tmp` names and hard-coded
  ports create false failures and can corrupt another run.
- Before starting a Docker-backed Dingo harness, render its Compose model and
  read the wrapper scripts through cleanup. Check running containers and
  networks for explicit names, fixed subnets, direct Docker name references,
  fixed volumes, and fixed temporary paths. A unique Compose project and host
  ports do not isolate those resources; leave the check unrun when they overlap
  a live devnet rather than risking the live evidence.
- Preflight generated-genesis conservation before waiting on a devnet: the
  configured maximum lovelace supply must cover every generated allocation,
  including delegated pool supply and generated wallets. Treat a startup that
  stops on this invariant as a fixture/harness failure, not as protocol or
  interoperability evidence.
- Do not run more than one Dingo sync or from-genesis load at once. Independent
  live `serve` checks may run concurrently when their ports and data paths are
  isolated. Remap every dependent NtN/NtC or metrics port, not just the main
  listener.
- For a long or background command, retain its output path, wait for the
  process to finish, inspect the complete log and exit code, and verify that
  the intended gate actually exercised the target test or artifact. A task
  notification is not the result. Rerun a failed or killed gate after fixing
  the cause, and compare branch results with an `origin/main` baseline when
  the failure could be environmental.
- Prefer the narrowest deterministic test first, then the repository-required
  race, lint, boundary, SQL/docs parity, conformance, and devnet checks. Keep
  baseline failures separate from regressions introduced by the branch.
- Before dispatching a cross-platform workflow, read the workflow files on the
  current base and confirm the selected workflow still has `workflow_dispatch`.
  Dispatch the exact pull-request branch, then verify the run's head SHA and
  rendered matrix job names. A retired workflow filename can resolve to stale
  metadata and fail with a misleading dispatch error; a hand-composed check
  context can differ from the matrix context GitHub actually reports.
- Do not assert strict ordering between two calls to the host wall clock.
  Windows can return equal timestamps for fast adjacent operations. For an
  ordering invariant, inject the clock and use an explicit channel or hook to
  prove the first event happened before the second; do not add a sleep to make
  the clock advance.
- Include a bounded benchmark smoke gate when performance fixtures are in
  scope. If one benchmark aborts its package, enumerate the declared benchmark
  functions and run exact-name cases individually so later cases are not
  silently absent. A benchmark labeled "real data" must assert that it seeded
  non-empty, relevant data; chain fixtures must preserve real block numbers and
  contiguity; hand-built ledger states must publish the snapshots required by
  the production path. Classify setup failures as benchmark debt unless the
  same panic or error is reachable through production construction and calls.

## Diagnose live and performance failures

- Preserve evidence before changing anything: logs, metrics, tip/slot gaps,
  selected peers, process state, and the exact revision/configuration. Filter
  only known benign noise; repeated warnings, dropped EventBus events,
  unexpected validation errors, and unexplained metrics outages are issue
  candidates, not acceptable catch-up noise.
- For sync plateaus, inspect chain-selection eligibility, peer freshness,
  stalled-client recycling, and fork/intersection history together. Do not
  preserve an incumbent peer after the selection code has filtered it out.
- For memory reports, capture CPU/heap profiles and identify the retaining
  path before changing cache or CBOR behavior. Distinguish a bounded cache or
  expected catch-up retention from an unbounded leak. When the fix replaces
  retained decoded values with compact or lazily decoded data, measure the
  adjacent recovery and lookup paths as well: a correct byte bound must not
  turn repeated peer-controlled input into an unbounded decode or scan cost.
  Keep a related CPU correction separately reviewable when it is not required
  for the memory invariant itself.
- For shutdown failures, trace the lifecycle context and reverse-order
  ownership of APIs, mempool, ledger/database, and storage. Reproduce with a
  focused test before changing timeouts or adding retries.

## Review control flow, not just comments

- Treat CodeRabbit, Cubic, and other review findings as hypotheses. Verify the
  exact current path and its preconditions before editing.
- Add the smallest test that proves the reported behavior, including the
  negative case. Examples from Dingo reviews include equal-height chain
  selection, an ineligible incumbent, an actually previous blockfetch
  connection, absence of a persisted gate, rejection of mutually exclusive
  flags, and a real reclassification assertion.
- Tests must model concurrency honestly: protect maps read by goroutines,
  avoid real DNS/network access in unit tests, register cleanup immediately,
  and use `WaitForCondition`, `RequireReceive`, `require.Eventually`, or
  context cancellation rather than `time.Sleep()`.
- Check scripts as contracts too: reject unknown arguments when appropriate,
  select the intended producer/reference node, honor configured ports, and
  fail if a named gate silently stops exercising its target test.

For Dingo cache and decoder reviews, inspect hot-entry TTL enforcement,
bounded retained memory, in-flight waiter wakeup, panic cleanup, and benchmark
error handling. A passing race run does not replace checking those contracts.

For Koios/account reviews, inspect exact amount validation before equality,
duplicate reference-row handling, coverage error classification, account
defaults through every composition path, and cancellation of remaining API
chunks after the first failure.

## Run a defensive whole-repository audit

For an audit that promises every declaration rather than a change review, use a
single append-only evidence ledger and an exact machine manifest. Include
functions, methods, every named type, function literals, generated code, tests,
examples, and nested support modules. Reconcile declaration rows to physical
files so build-tagged and zero-declaration files cannot disappear. Assign each
row to one review slice and close the manifest with zero unassigned rows before
making an exhaustive-coverage claim.

Verify every Dingo dependency pin by commit/tag identity and by the source root
recorded in its inventory. Directory names such as `pinned`, an existing
checkout, and a green maintained-head suite are not proof of the build pin.
Review the exact pin first and label maintained-head behavior as a separate
compatibility delta. Exercise nested/example modules under their own Go graphs.

Use later validation as a controlling disposition rather than erasing earlier
evidence. For consensus or ledger behavior, include a valid control, the
malformed or boundary case, and the opposite direction of any flag or state
transition. Distinguish rule-level proof, running-Dingo proof, and differential
proof against `cardano-node`; say explicitly when a funded or otherwise usable
devnet was unavailable.

Treat raw severity headings as evidence records, not unique defect counts.
Machine-check finding IDs and any prior issue/quality-ledger crosswalk for
duplicates and omissions, and list the controlling unresolved subjects in the
handoff. Check issues and pull requests read-only unless the task authorizes
tracker changes. Keep private findings out of shared skills, public docs, logs,
image labels, and command-line arguments.

Prefer exact `ghcr.io/blinklabs-io` toolchain and Cardano images, with their
resolved digests recorded. Use unique run-owned worktrees, caches, paths, ports,
and Docker names. After the evidence is durable, remove only those owned
artifacts and verify the live producer/devnet and unrelated concurrent work are
unchanged.

## Prove consensus behavior against a reference

A claimed divergence is only as good as its oracle. Rank the proof: rule-level
(drive the exact exported gouroboros/plutigo API the pinned Dingo links, via a
`replace` directive in a throwaway module), running-Dingo (in-process apply
path, or a live node), and differential against `cardano-node`. State which one
each finding rests on.

- The reference oracle is local. `internal/test/devnet/run-tests.sh
  --conformance` brings Dingo up beside real `cardano-node`; the same
  `ghcr.io/blinklabs-io/cardano-node` image's `cardano-cli` decodes and
  validates crafted scripts, certs, and transactions on its own, which settles
  "does the network accept these bytes" without a full devnet. Record the image
  digest and the cardano-cli version.
- Submit identical bytes to both nodes over LocalTxSubmission and diff the
  verdicts; `cardano-node`'s error names the exact rule. Always run the honest
  control (well-formed input accepted by both) so a rejection is attributable
  to the crafted input, not a broken harness. Devnet genesis funds are
  finite — a single UTxO-consuming run can strand the network, so budget
  funding or fall back to the rule-level proof rather than rebuilding.
- 32-bit narrowing claims are testable without a 32-bit host when
  `qemu-*-static` is registered in binfmt_misc: `GOARCH=386 go test` builds and
  executes real 386 binaries. If the node itself will not compile for the
  target, the narrowing is library-only for third-party consumers — say so and
  scope the severity down rather than leaving it theoretical.
- A reproduction task is defensive robustness testing of first-party code, not
  offense. Frame it that way (a regression test that fails without the fix, an
  input the decoder should reject); "prove a remote process kill" phrasing can
  trip content safeguards and stall the work with nothing gained.

## Recurring risk classes to check first

These invariants have repeatedly failed to hold across Dingo and its
dependencies; verify each one holds rather than assuming it, and phrase notes as
invariants to confirm, not as disclosed exploits.

- Panic reachability is decided by the goroutine, not the panic. `net/http`
  recovers a panic on its own handler goroutine (the connection dies, the
  process lives); a goroutine the handler *spawns* is not covered, and neither
  are the per-connection Ouroboros loops or the ledger/forge goroutines. Audit
  request- and network-reachable paths for a panic on an unrecovered goroutine.
- A trust anchor must fail closed. A missing or empty genesis key, verification
  key, genesis hash, or protocol coefficient must halt or hard-error, never
  silently downgrade or disable the check.
- Judge a listener's default posture as a set: bind address, auth, TLS, and
  CORS. Public + unauthenticated + wildcard CORS + a mutating endpoint is the
  compound risk, and the defaults are often shared across several listeners
  through one config layer — fix them at that layer once.
- Enforce lengths and counts at decode. Wire- or chain-derived bytes copied
  into fixed-size arrays silently pad or truncate (a reflection-decoded fixed
  hash type with no explicit length check is the usual source); cross-array
  count invariants (for example witness sets versus transaction bodies) must be
  checked before any accessor indexes them, because a body-hash check does not
  catch a count mismatch.
- A consensus decoder must be exactly as strict as the reference: reject the
  non-canonical CBOR the reference rejects, and never let a non-shortest-form
  length header be read as a union discriminant.
- Re-run, do not trust, a declared outcome. A validating node must re-evaluate
  phase-2 scripts and reject a block whose declared validity flag disagrees with
  the actual result in either direction, and must not gate whole rule sets on a
  flag the peer controls.
- Bound per-peer retention by bytes and by connection lifetime, not by entry
  count alone, and charge script/CPU budgets against the real protocol limit
  rather than a placeholder maximum.

## Leave durable follow-up

- Update `ARCHITECTURE.md` for component, lifecycle, event, plugin, or
  concurrency changes and `DATABASE.md` for schema, migration, query, blob,
  encoding, or durability changes. Say explicitly when each was checked and
  unaffected.
- File an issue for a confirmed flaky test, dropped event, missing invariant,
  or infrastructure limitation instead of hiding it in a filtered log or a
  local plan. Plans remain local and ephemeral.
- Use a Conventional Commit with DCO sign-off for each focused change, and
  report the exact checks, baselines, skipped infrastructure, and remaining
  follow-up in the PR.
