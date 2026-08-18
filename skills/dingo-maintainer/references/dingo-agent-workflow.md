# Dingo agent workflow

Use this reference with the Dingo maintainer skill when investigating a live
failure, a large validation run, or a review finding. It distills recurring
patterns from Dingo's repository guidance and agent sessions; the Dingo
checkout remains the source of truth for commands and behavior.

## Start from a known state

- Check current `origin/main`, open issues, and open pull requests before
  proposing a fix. Reuse an existing issue or PR when the work overlaps.
- Start implementation or review work in a fresh worktree based on the
  intended remote base. Keep the main checkout, other worktrees, and existing
  `.claude/` or other agent state intact.
- Split unrelated fixes into separate, focused branches and PRs. Do not rewrite
  history to make the result look cleaner.
- If a live node, devnet, or validation run is in progress, treat it as an
  observation target. Do not stop, restart, reconfigure, or delete its data
  unless the task explicitly authorizes that intervention.

## Run validation reproducibly

- Treat required checks as required. If a check cannot run, record the exact
  command and reason; do not silently skip it.
- Use unique ports, temporary directories, profiling databases, logs, and
  `GOCACHE` values for concurrent runs. Fixed `/tmp` names and hard-coded
  ports create false failures and can corrupt another run.
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
  expected catch-up retention from an unbounded leak.
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
