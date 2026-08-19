---
name: isolated-validation-runs
description: Run Blink Labs builds, tests, devnets, and long validation jobs without corrupting the main checkout or colliding with other runs. Use for worktree setup, unique ports and temporary paths, background task discipline, serialized sync runs, flaky-failure triage, and baseline comparison against origin/main.
---

# Isolated Validation Runs

Use this skill whenever a check is slow, stateful, networked, or run more than
once concurrently. Most wasted validation time in this workspace comes from two
mistakes: running in the wrong tree, and trusting a task notification instead of
the output.

## Isolate the tree

1. Start from current `origin/main` unless the task says otherwise. Fetch first;
   a stale base makes both the failure and the fix untrustworthy.
2. Use a dedicated worktree or branch for implementation and review work. Leave
   the user's main checkout, its untracked files, and any local agent state
   (`.claude/`, `CLAUDE.md`, roadmaps, scratch directories) exactly as they are.
3. Never reuse one worktree for two concurrent runs. The build cache, database
   directory, and socket paths are shared state.
4. When finished, report where the worktree is rather than silently removing it
   if it still holds evidence the user may need.

## Isolate the resources

Every long-running check needs its own:

- port range — never the default when another run may be live;
- temporary directory — a unique path per run, not a shared `/tmp/test`;
- `GOCACHE` and any database or ledger directory;
- log file, captured to disk rather than only to the terminal.

Serialize the runs that cannot be parallelized: sync-from-genesis, devnet, and
anything binding a fixed port or writing a fixed database path. Two of those at
once produce failures that describe the collision, not the code.

## Give slow suites a timeout that fits

`go test` defaults to a 10-minute per-package timeout, and packages in this
workspace exceed it: Dingo's `ledger` package runs 9-13 minutes under `-race`,
and its CI uses `-timeout 20m` for the whole tree. A run killed by the default
timeout reports `panic: test timed out` and a non-zero exit that looks exactly
like a real failure, and the test named in the panic is merely the one running
when the alarm fired — not the cause.

Pass an explicit `-timeout` sized to the suite before concluding anything from a
timeout, and match CI's value when you have it. Then confirm the failure
reproduces on an `origin/main` baseline before attributing it to the change.

## Match CI's build tags

Build tags decide which files exist. A package can compile and pass with no
tags and fail to build with the repository's default set, so a run without them
is not the run CI performs. Dingo's Makefile defaults to
`BUILD_TAGS=dingo_extra_plugins` and threads it into every `go` invocation;
`go test ./...` on its own silently checks a different program.

Read the Makefile for the tag set before running anything by hand, and prefer
the repository's own targets. This matters most right after a merge or rebase: a
signature change on one side and a call site on the other can compile cleanly
under one tag set and break under another, so the error appears only when the
tags match CI's.

## Background task discipline

A completion notification is not a result. For every backgrounded or long check:

1. Wait for the process to finish.
2. Read the complete output, not the tail.
3. Confirm the exit code **in the log**. A completion notice can report success
   for a run whose log ends in `FAIL` and a non-zero exit; the notice reports on
   the process, not on the tests.
4. Confirm the intended test or gate actually ran — a suite that skipped
   everything exits zero.
5. Check the run started **after** your last edit. A background job compiles the
   tree as it found it, so its log describes that tree, not the current one.
   Editing a file mid-run makes the output stale in a way nothing in it
   announces — and the trap runs both directions: a stale failure invites
   dismissing a real defect, and a stale pass invites shipping one. When in
   doubt, stop the run and start it again rather than reasoning about which
   files it saw.

Do not stop, restart, or reconfigure a live node or an in-flight validation run
while diagnosing it unless the task explicitly authorizes that intervention.
The running state is usually the only evidence.

## Triage a failure before attributing it

1. Reproduce it. A failure seen once is a report, not a finding.
2. Re-run the same command on an `origin/main` baseline in a separate worktree.
   A failure present on both is pre-existing and must be reported as such.
3. Check whether the failure is in generated code, in a nested module the root
   command does not cover, or in an environment-dependent gate.
4. For a suspected flake, run the focused test repeatedly with `-count` and with
   `-race` before concluding anything.
5. Never quiet a flake by filtering, skipping, retrying, or sleeping. Confirmed
   flakes and dropped events become issues in the owning repository.

Never use `time.Sleep()` to synchronize a test. Use the repository's wait
helpers — in Dingo, `internal/test/testutil/WaitForCondition` and
`RequireReceive` — or a context with a timeout.

## Preserve the evidence

Keep the command line, working directory, environment overrides, exit code, log
path, and relevant output for every run. Preserve failing artifacts — logs,
databases, cores, captured chain state — before retrying, because the retry
usually destroys them. Report what you kept and where it is.
