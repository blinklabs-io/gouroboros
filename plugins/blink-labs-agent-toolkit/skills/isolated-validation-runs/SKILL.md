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

Before starting a Compose or shell-driven container harness, inspect both the
live Docker resources and the fully expanded harness. Look for explicit
`container_name` values, fixed networks/subnets, direct `docker ... <name>`
calls, fixed volumes and temporary paths, and cleanup commands that address a
resource outside the Compose project. `COMPOSE_PROJECT_NAME` and host-port
overrides do not isolate any of those resources. If an explicit name, subnet,
or cleanup target overlaps a live run, do not start the harness; record the
collision and use a genuinely isolated harness or leave that check unrun.

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

Two CI jobs can also disagree with each other. Dingo's `go-test (Linux)` passes
`-tags dingo_extra_plugins` while `go-test-windows` runs bare `go test ./...`,
so a test file that omits the constraint carried by the file it tests compiles
in both jobs and only fails in the one without the tag. That is how
`undefined: NewWithOptions` kept `go-test-windows` red across several commits
while every Linux run stayed green.

A test file belongs to the same build as its subject: if `database.go` is behind
`//go:build dingo_extra_plugins`, so is its test. Reproduce that class without
the other platform by dropping the tag — `go vet ./...` with no tags is the
Windows job's shape, and it names the undefined symbol immediately.

## Prefer published Blink toolchain images

When Docker is useful, look for the exact toolchain under
`ghcr.io/blinklabs-io` before building a local image. Record the requested tag,
resolved manifest digest, image ID, and platform in the validation evidence;
tags alone are mutable and an amd64 image under emulation is not equivalent to
a native arm64 run.

For Go validation, prefer `ghcr.io/blinklabs-io/go:<exact-tag>` and set
`GOTOOLCHAIN=local` so the command proves the image's toolchain rather than
silently downloading another one. Mount or create writable, run-owned
`GOCACHE`, `GOMODCACHE`, and temporary paths, and test each nested module or
example under its own `go.mod`/declared dependency graph.

If no suitable published image exists, give a locally built fallback a unique
audit/run name and label. Record its base, Dockerfile or build context, image
ID, and platform, then remove it with the rest of the run-owned artifacts.
If cross-architecture emulation crashes, hangs, or stops making progress,
terminate it and report the check as not run. Static review or reference vectors
do not turn an emulation failure into executable conformance evidence.

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

A pipeline can manufacture both halves of a false result. `go test ... | grep -E
… | head -20` reports **`head`'s** exit status, not the suite's, and closes the
stream early so the log stops before the failures and before the trailing
`ok`/`FAIL` line. The output then shows a run of passes and an exit code of
zero for a suite that never finished. Redirect to a file and read that, or put
the filter after the status check — never let `head`, `tail` or an early-closing
consumer sit at the end of a pipeline whose exit code is being trusted.

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

Benchmark suites need the same treatment. An early package panic or fatal can
prevent every later benchmark from running, so enumerate the declared
benchmarks and rerun them individually before claiming benchmark coverage.
Use a bounded smoke setting such as `-run '^$' -bench . -benchtime=1x` first;
then classify setup failures separately from behavior reached through the
production constructors and call graph.

Never use `time.Sleep()` to synchronize a test. Use the repository's wait
helpers — in Dingo, `internal/test/testutil/WaitForCondition` and
`RequireReceive` — or a context with a timeout.

Synchronising on time and *asserting* on it are separate problems. For a test
that measures elapsed time and compares it, see
[regression-test-discipline](../regression-test-discipline/SKILL.md): such a
test states something about the host as much as the code, and needs a control
and a guard for a clock too coarse to resolve the work.

## Preserve the evidence

Keep the command line, working directory, environment overrides, exit code, log
path, and relevant output for every run. Preserve failing artifacts — logs,
databases, cores, captured chain state — before retrying, because the retry
usually destroys them. Report what you kept and where it is.

## Clean up run-owned artifacts

Create an ownership manifest as the run starts: worktrees, temporary roots,
ports, containers, images, volumes, networks, caches, logs, and downloaded
fixtures. Clean those artifacts after their evidence has been summarized and
hashed. Cleanup is part of the run's definition of done, not an unbounded
workspace purge.

Before deletion, check process command lines, container status, worktree status,
and current paths. Remove a clean temporary Git worktree through `git worktree
remove`, not by deleting its directory. Go module caches can contain read-only
files; make only the exact run-owned cache user-writable before removing it.

Never delete a live devnet/node, a concurrent run's cache or worktree, user
changes, credentials, or a shared/pre-existing Docker image merely because it
is reclaimable. Prefer exact paths or a reviewed run-specific prefix over broad
globs. Report the artifacts removed, the evidence deliberately preserved, live
services checked afterward, and disk usage before and after cleanup.
