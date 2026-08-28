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

2. **Dispatch domain review in parallel** with the toolkit subagents, scoped to
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

3. **Reconcile bot findings.** Check each one against the branch's current head
   first — on an active branch most open findings are already fixed and the
   thread is simply unanswered. For the rest: locate the exact current path and
   symbol, check the repository's local rules and existing tests, then reproduce
   or disprove it with a focused test or a direct control-flow reading. Classify
   as merge blocker, non-blocking recommendation, false positive, or already
   addressed. Do not copy bot prose into durable documentation.

4. **Fix the cause, not the sentence.** A finding names a symptom at one
   location; the defect is often the boundary it sits on. Before changing a
   response shape, an error path, or a return contract, read the consumer —
   `cross-boundary-changes`. Decide the contract once and hold it: reshaping the
   code on each round of feedback costs a review cycle every time.

5. **Verify the negative case.** For every confirmed behavioral finding, check
   that a test exists for both the reported behavior and its absence case, and
   that the new test fails without the fix — `regression-test-discipline`.

6. **Check the change bar.** For Dingo, `DATABASE.md` and `ARCHITECTURE.md` are
   part of the change; state whether they were updated or checked and unaffected.
   For API changes, check generated output, docs, and downstream callers in the
   workspace.

7. **Follow the target's review gates.** For Blink repositories, run or wait for
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

## Report

Order findings: behavioral and security defects, then API or wire
compatibility, then missing contract-specific tests, then architecture
boundaries, then documentation and generated drift, then style. For each
finding give the path, current line or symbol, the evidence that confirmed it,
and the classification. Keep the PR-level summary short and factual; put
code-specific feedback in inline comments. End with unresolved risks and the
skipped-check ledger.
