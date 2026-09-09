---
name: regression-test-discipline
description: Decide whether a fix warrants new coverage and, when it does, write a test that actually fails without the fix and prove it. Use when adding a test for a bug fix or review finding, when a proposed test would only forbid a deleted static value, when a test asserts something the test itself just performed, when sequencing a concurrency test, or before claiming a fix is covered.
---

# Regression Test Discipline

A test added alongside a fix has one job: fail without the fix. Most tests that
fail that job still pass, which is why the job has to be checked rather than
assumed.

## Decide whether a new test is warranted

A fix does not automatically require a new test. Do not add a source assertion
whose only purpose is to forbid a deleted URL, string, customer, or other static
content value from returning. That turns a one-off content decision into a
permanent product invariant and tests the current source text instead of
behavior.

For static content cleanup, preserve the valid surrounding data and run the
repository's existing type, format, build, link, and content validation. Add a
regression test only when the change establishes durable behavior or a contract
that existing checks do not cover. If rendered behavior and public contracts
are unchanged, existing validation can be the complete test plan.

## Prove it fails

For every test added with a fix:

1. Revert the fix (keep the test).
2. Run the test.
3. Confirm it **fails**, and that the failure message names the actual defect.
4. Restore the fix and confirm it passes.

Then check the check: a reverted build that does not compile proves nothing. If
step 2 reported a build failure rather than a test failure, the verification did
not run — repair the revert so the package compiles, and do it again. The
temptation to accept "it errored, close enough" is exactly where a useless test
gets committed.

### Fail-before must reach the intended assertion

A fixture-selection failure, setup error, timeout, network error, or empty test
filter is not fail-before evidence. Confirm the test reaches the action under
review and fails at an observable contract mismatch whose message names the
defect. Record that assertion and message; otherwise a later setup repair can
silently turn the supposed regression into a test of something else.

Do not respond to a sparse fixture corpus by scanning an ever-larger prefix in
the regression itself. Prefer a bounded deterministic fixture plus a narrow
injection seam. Preserve the identity and traversal fields that drive the
production path, substitute only the minimal payload needed to expose the
behavior, document that separation, and leave shared fixtures unchanged. Then
repeat the old-code run and reject the proof unless it reaches the intended
assertion.

Keep fail-before output bounded and diagnostic. For a cardinality, allocation,
or retained-byte regression, aggregate the observed result and assert once
after the workload. Do not place a large object equality or byte-slice assertion
inside a long loop: the correct red run can otherwise emit megabytes of repeated
diffs, obscure the first contract failure, and make useful baseline evidence
hard to retain.

## The tautology trap

A test that performs a sequence and then asserts that same sequence happened
cannot catch a regression in the code it names. The shapes to watch for:

- The test hand-runs the production path's steps (close, nil, set a flag) and
  then asserts those steps' results.
- The test asserts on the helper the production code happens to call, rather
  than on the value the production code produces. Assert `state.CanClaim`, not
  `datum.IsMatured()`.
- The assertion is satisfied by a guard *earlier* than the one under test — a
  push refused by a later signature check passes a test that meant to prove an
  earlier depth check ran.
- The assertion already held **before** the action under test ran, so it says
  nothing about the action. See below.

When several rules could reject the same input, assert on **which** rule
rejected it: the log line, the specific error, or an observable that only the
rule under test produces. Otherwise the test passes with the fix removed.

## Check the assertion's starting state

Before trusting an assertion, ask what it would have reported *before* the
action under test. If it would already have passed, it is not testing anything.

The common shape is a counter or flag that the test's own setup already put into
the expected state:

```go
// handled is already 1 from the in-flight event, so this passes on its first
// poll and never observes whether the publish below was delivered.
eb.Publish(evtType, event.NewEvent(evtType, "after-unsubscribe"))
require.Eventually(t, func() bool { return handled.Load() == 1 }, ...)
```

`Eventually` proves a state is *reached*. When the point is that a state does
not change, that is `Never` on the negation:

```go
require.Never(t, func() bool { return handled.Load() != 1 }, ...)
```

Same rule for `assert.Nil` on something never populated, or a "no error"
assertion on a path that cannot error yet. Reverting the fix catches most of
these — an assertion that was already true stays true — which is another reason
the revert step is not optional.

## A failing test must fail, not hang

A test that blocks a goroutine to create a window has to release it on every
exit path, including the failing one. Otherwise the deferred `Close` waits on
the still-blocked handler, and instead of a legible failure the run dies on the
package timeout — which reads as infrastructure trouble rather than the defect
the test just caught.

Register the release **after** the deferred teardown so it runs before it, and
make it idempotent:

```go
defer eb.Close()
var releaseOnce sync.Once
release := func() { releaseOnce.Do(func() { close(releaseCh) }) }
defer release() // runs before eb.Close()
```

`t.Cleanup` does not solve this: cleanups run after the test function's defers,
so a deferred `Close` still blocks first.

## Assert at the moment the contract promises

When a fix moves work into a goroutine or a later call, the test has to check
the state at the instant the contract claims it holds — not eventually. "`Stop`
returned" and "the port is free" are two different moments, and a fix that only
starts the release before returning has narrowed the window, not closed it.

Write the assertion immediately after the call, with no polling and no sleep:

```go
require.NoError(t, srv.Stop(ctx))
require.False(t, portAccepts(addr)) // not Eventually: Stop returning must mean released
```

If that cannot hold without the caller waiting for another goroutine, the fix is
incomplete — the release needs to be something the call itself waits for. A
loop of many iterations is worth it here: these races surface at a few percent
per attempt, so a single pass passes on a broken build.

## Sequencing concurrency tests

Do not build a test's ordering on unspecified runtime behavior. Mutex fairness,
starvation-mode handoff, goroutine scheduling order, and map iteration order are
implementation details, not guarantees, and a test resting on them is fragile
even when it passes today.

Use, in order of preference:

1. An explicit test hook on the production path — a nil-by-default function
   field the production code calls at the point the test needs to observe. It is
   a few lines of production code and it makes the ordering a stated contract.
2. Channel handshakes between the test and the code under test.
3. The repository's wait helpers (in Dingo, `WaitForCondition` and
   `RequireReceive`) or a polling assertion on a real condition.

Never `time.Sleep()` to synchronize. Assert the invariant directly where you
can: "the mutex is acquirable while Close waits" is checkable without ordering
two goroutines at all.

## A test that can observe zero is not synchronised

A non-blocking read is not a wait. `select` with a `default` arm, a `len(ch)`
check, or a drain loop that returns as soon as the channel is momentarily empty
all read whatever happens to be there *now* and return immediately when nothing
is. Pair one with a count assertion and the test asserts on a race:

```go
// Broken: returns instantly if the producer has not run yet.
for {
    select {
    case evt := <-ch:
        out = append(out, evt)
    default:
        return out
    }
}
...
require.Len(t, out, 1)   // reports "0" when the test merely looked too early
```

There is no happens-before edge between the producer and the assertion, so the
test does not fail when the code is wrong — it fails when the scheduler is
unkind. It passes while the producer reliably wins the race and starts failing
when anything changes the odds: a new sibling `t.Parallel()`, a slower runner, a
busier CI host. Nothing about the test changed at that point; only the odds did.

The tell is the failure message. `should have 1 item(s), but has 0` and
`"0" is not positive` are the shape of an unsynchronised read, not of a
regression — a genuinely broken producer usually publishes the *wrong* event,
not none at all. Treat a zero-observation failure as a missing wait until you
have proved otherwise.

Block on the event instead: `RequireReceive` (Dingo's helper) or a bare
`case <-ch:` against a timeout, waiting for each event the scenario promises,
then drain for unexpected extras. Assert "the first event is X" by receiving it,
and "there are no others" by draining after — those are two different
assertions and only the second may use `default`.

This is the failure mode behind dingo#4145: two frontier-flap tests drained
non-blockingly and asserted a count, passed on every platform for as long as
they ran alone, and turned `main` red for hours once independent tests began
running in parallel.

## Make the unreachable reachable

A branch that cannot be driven from a test is a branch nobody has verified. When
the failure needs an error that the surrounding locking makes impossible,
introduce a narrow seam rather than replicating the branch's body in the test:

- a package-level function variable the test swaps and restores;
- an injectable interface on the struct;
- a test-only hook field, documented as such.

Say in the comment why the branch is otherwise unreachable, so the seam is not
mistaken for indirection with no purpose.

## Cover the negative case

For every rule a fix adds, test both that the valid input is accepted and that
the invalid one is refused. A fix that only proves the happy path still works
has not been tested at all.

For an interval, threshold, or era boundary, derive equality behavior from the
governing specification or reference implementation before writing the test.
Exercise the value below, exactly at, and above the boundary; otherwise a test
can permanently encode the same guessed comparator as the bug.

Also drive the rule through the production entry point that sequences validators,
not only through its helper. In Go error aggregation, appending a `nil` error and
then branching on `len(errs)` can call `errors.Join(errs...)`, return `nil`, and
skip every validator that follows. Add a control proving that a later validator
still runs when the new rule succeeds, and append only non-nil errors.

For a multi-phase defect, prove the transition that consumed the bad state, not
only the phase that stored it. A row query or direct helper call can show that a
prerequisite exists while missing the same rollover, replay, or restart failure
operators see. Build the smallest complete eligible input, drive the real
production transition, and assert its downstream effect. Keep lower-level
atomic and exact-byte tests as supporting coverage; do not use empty eligibility
markers as a substitute for proving the stored bundle is consumable.

## A test that measures duration is measuring the machine

An assertion on elapsed time states something about the host as much as the
code, so say which part is the claim.

**Never assert an absolute threshold or a raw growth ratio.** One sweep asserting
`ratio < 1.5` produced 2.6x on CI and 0.45x on a developer machine from the same
commit: it was measuring page cache and commit behaviour, not the index under
test. Compare against a control built in the same process that differs only in
the thing being tested, and read the comparison where the signal is widest —
usually the largest input, not the smallest, because that is where the effect
clears the timing floor by the largest margin.

**A ratio of two noisy samples is noise.** A first-to-last ratio of one store's
own measurements lands either side of 1 from run to run. Take both terms at the
same position so both carry whatever contention the run is under; a maximum
taken across phases imports the single most contended phase into a comparison
against a term measured elsewhere.

**Handle a clock too coarse to resolve the work.** On a platform whose timer
granularity exceeds the operation, every sample is zero, a ratio of zeroes is
NaN, and an ordering assertion reports something like
`Can not compare type "float64"` — which reads as a type bug rather than as an
unmeasurable run. Detect the zero sample and skip, naming the input size and the
samples. Raising the workload until every platform clears its floor slows the
suite everywhere to keep one platform honest, and asserting anyway reports a
timer resolution as a regression in the code.

**Run the whole package, not the filtered test.** A `-run` filter is quiet in a
way the real suite is not: sibling `t.Parallel()` tests are the contention these
measurements pick up, and a threshold calibrated without them is calibrated
against a machine nobody has.

## Do not weaken a test to make it pass

When an existing test fails after a fix, decide which is wrong. If the fix is
right, update the test *and its comment* to state the new expected behavior. If
the assertion was checking something real, the fix needs revisiting. Deleting an
assertion to get green is how a covered behavior silently becomes uncovered.
