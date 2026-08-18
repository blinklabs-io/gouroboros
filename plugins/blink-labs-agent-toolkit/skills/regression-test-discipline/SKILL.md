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

When several rules could reject the same input, assert on **which** rule
rejected it: the log line, the specific error, or an observable that only the
rule under test produces. Otherwise the test passes with the fix removed.

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
