---
name: regression-test-discipline
description: Write a test that actually fails without the fix, and prove it. Use when adding a test for a bug fix or review finding, when a test asserts something the test itself just performed, when sequencing a concurrency test, or before claiming a fix is covered.
---

# Regression Test Discipline

A test added alongside a fix has one job: fail without the fix. Most tests that
fail that job still pass, which is why the job has to be checked rather than
assumed.

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

## Do not weaken a test to make it pass

When an existing test fails after a fix, decide which is wrong. If the fix is
right, update the test *and its comment* to state the new expected behavior. If
the assertion was checking something real, the fix needs revisiting. Deleting an
assertion to get green is how a covered behavior silently becomes uncovered.
