---
description: "Review a Blink Labs or TosiDrop change using the target organization's rules, repository-aware findings, current-head bot state, and required human review"
argument-hint: "[PR number, branch, or path]"
allowed-tools: ["Bash", "Glob", "Grep", "Read", "Task", "WebFetch"]
---

# Owned-repository review

Target: "$ARGUMENTS" (if empty, review the current branch diff against its base).

Use `github-review-coordinator` for discovery and current-head state, the
organization maintainer skill for policy, and domain skills for judgment. Bot
findings are review inputs, never conclusions.

## Steps

1. **Establish the owner, diff, and boundary.** Identify the GitHub owner, base
   branch, changed repositories, changed nested modules, and current head SHA.
   Load `blink-repo-maintainer` for `blinklabs-io` or
   `tosidrop-repo-maintainer` for `TosiDrop`. Do not import one organization's
   DCO, screenshots, bot order, merge ownership, or release rules into the
   other.

2. **Read the linked issue and everything it links to.** Follow the issue the
   pull request claims to close, plus the links, specs, and upstream references
   that issue carries. Confirm the change actually addresses it and that
   everything in the diff is relevant to it; scope the PR does not need is a
   finding, and so is an issue reference that names work a different change
   implements.

3. **Dispatch domain review in parallel** with the toolkit subagents, scoped to
   the parts of the diff each one owns:
   - `blink-protocol-auditor` for ledger, CBOR, Ouroboros, Plutus, consensus,
     and conformance fixtures;
   - `blink-app-auditor` for wallet, key, transaction, DEX, and indexer behavior;
   - `blink-go-module-auditor` for `go.mod`/`go.sum`, replacements, and module
     identity;
   - `blink-release-auditor` for Dockerfiles, manifests, and publishing
     workflows;
   - `blink-repo-scout` when the diff's callers or fixtures need to be traced
     across repositories.

4. **Reconcile bot findings.** Check each one against the branch's current head
   first — on an active branch most open findings are already fixed and the
   thread is simply unanswered. For the rest: locate the exact current path and
   symbol, check the repository's local rules and existing tests, then reproduce
   or disprove it with a focused test or a direct control-flow reading. Classify
   as merge blocker, non-blocking recommendation, false positive, or already
   addressed. Do not copy bot prose into durable documentation.

5. **Fix the cause, not the sentence.** A finding names a symptom at one
   location; the defect is often the boundary it sits on. Before changing a
   response shape, an error path, or a return contract, read the consumer —
   `cross-boundary-changes`. Decide the contract once and hold it: reshaping the
   code on each round of feedback costs a review cycle every time.

6. **Verify the negative case.** For every confirmed behavioral finding, check
   that a test exists for both the reported behavior and its absence case, and
   that the new test fails without the fix — `regression-test-discipline`.

7. **Check the change bar.** For Dingo, `DATABASE.md` and `ARCHITECTURE.md` are
   part of the change; state whether they were updated or checked and unaffected.
   For API changes, check generated output, docs, and downstream callers in the
   workspace.

8. **Follow the target's review gates.** For Blink repositories, run or wait for
   configured bot reviews, address actionable findings, rerun affected checks,
   then request the required human review. For TosiDrop, discover the target's
   configured checks, approvals, and bots first; a green CodeRabbit status that
   says review was skipped or needs a manual trigger is not a completed review.
   Bot approval or silence is never human approval.

   In Blink's configured bot loop, reply to every bot thread, including
   already-fixed and rejected ones, and expect one more bot pass after a push.
   In TosiDrop, follow the target's actual bot configuration and do not treat a
   skipped review as a clean pass.

   Apply author-only merge handling only where the target organization or
   repository requires it. A review request alone authorizes inspection and a
   report, not posting a review or merging the pull request.

## What to look for

Be adversarial. Every line in the diff should have a reason to exist and hold
up to scrutiny; a change nobody can justify is a finding even when it works.
Beyond the domain skills' own concerns, always check:

- **Duplicated code** — logic the diff repeats from elsewhere in the repository,
  and logic it repeats within itself. Per-era ledger rules are the common case:
  a fix applied to one copy usually leaves its siblings wrong.
- **Tests actually pass**, and the new ones fail without the fix. Run them; do
  not infer them from a green check that may be a bot reporting it never ran.
- **Performance degradation** — added allocations or work on a hot path, an
  unbounded scan, an N+1 query, a per-request cost that scales with chain or
  UTxO size.
- **Concurrency** — deadlocks, lock-order inversions, goroutine leaks, data
  races, and state published before the write that backs it commits. Run the
  affected packages under `-race`.

## Verdict and posting

State a recommendation: approve, or do not approve. Do not end a review with an
unranked list of observations and no position.

Be explicit when the pull request should change state, and name the state —
`APPROVED`, `CHANGES_REQUESTED`, `COMMENTED`. Reserve `CHANGES_REQUESTED` for
actionable blockers; do not escalate trivial recommendations into one. A red or
still-running pipeline withholds approval even when the diff is clean, which is
a `COMMENTED` review that says so, not a change request.

When the recommendation is not to approve, enumerate the exact comments to
leave, each anchored to its file and line.

**Do not post anything to GitHub without being told to.** A request to review is
authorization to inspect and report, not to publish. Post only on explicit
instruction, and when posting prefer inline comments on the exact lines over
PR-level prose, keeping the summary comment short.

Write for the person who will read it. Reviews and replies go to humans who may
be sensitive about their work, so be courteous and specific: name the problem
and the evidence, not the author's judgment. Adversarial about the code is the
goal; adversarial about the person is not.

## Report

Order findings: behavioral and security defects, then API or wire
compatibility, then missing contract-specific tests, then architecture
boundaries, then documentation and generated drift, then style. For each
finding give the path, current line or symbol, the evidence that confirmed it,
and the classification. Keep the PR-level summary short and factual; put
code-specific feedback in inline comments. End with unresolved risks, the
skipped-check ledger, and the approve / do-not-approve recommendation with the
pull-request state it implies.
