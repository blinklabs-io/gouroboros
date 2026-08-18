---
description: "Run the Blink Labs review loop on a change or pull request: repository-aware findings first, bot findings reconciled, human review requested last"
argument-hint: "[PR number, branch, or path]"
allowed-tools: ["Bash", "Glob", "Grep", "Read", "Task", "WebFetch"]
---

# Blink Labs review

Target: "$ARGUMENTS" (if empty, review the current branch diff against its base).

Use `github-review-coordinator` for sequencing and the domain skills for
judgment. Bot findings are review inputs, never conclusions.

## Steps

1. **Establish the diff and the boundary.** Identify the base branch, changed
   repositories, changed nested modules, and whether the parent workspace only
   records a submodule pointer.

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

7. **Sequence the humans last.** Run or wait for configured bot reviews, address
   actionable findings, rerun affected checks, then request the required human
   review through GitHub. Bot approval or silence is never human approval.

## Report

Order findings: behavioral and security defects, then API or wire
compatibility, then missing contract-specific tests, then architecture
boundaries, then documentation and generated drift, then style. For each
finding give the path, current line or symbol, the evidence that confirmed it,
and the classification. Keep the PR-level summary short and factual; put
code-specific feedback in inline comments. End with unresolved risks and the
skipped-check ledger.
