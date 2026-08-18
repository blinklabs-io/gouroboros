---
name: github-review-coordinator
description: Coordinate Blink Labs pull-request reviews across CodeRabbit, Cubic, and required human reviewers, including bot-first sequencing, fix loops, reviewer re-requests, and documented review dismissal. Use when preparing, updating, or handing off a GitHub pull request.
---

# GitHub Review Coordinator

Use this skill for pull-request review workflow, not for deciding whether a
code finding is technically correct. Read the [review loop reference](references/review-loop.md)
and the target repository's contribution guidance.

## Review sequence

1. Confirm the PR, base branch, changed repositories, checks, CODEOWNERS, and
   required reviewers. Exclude draft PRs. If the user excludes Dependabot,
   exclude PRs authored by `dependabot[bot]`.
2. Run or wait for configured CodeRabbit and Cubic reviews before requesting
   human review. Reproduce findings against the current checkout.
3. Address actionable bot findings, document false positives or accepted
   risks, rerun affected checks, and update the PR.
4. Request the required human review. Human review is mandatory and may be
   AI-assisted; bot approval or silence is never sufficient.
5. If a human reviewer requests changes, implement and validate the fixes,
   summarize changed paths and tests on the PR, and explicitly request another
   review from that same person through GitHub.
6. An authorized human reviewer may dismiss another human review when
   appropriate. Record the rationale on the PR and retain appropriate human
   review coverage.

Use GitHub's UI or API for reviewer requests and dismissal. Do not claim a
review, approval, re-request, or dismissal occurred unless GitHub shows it.
Separate bot findings, human findings, unresolved risks, skipped checks, and
final approval state in the handoff.

Before posting a review, verify the current head SHA, author, draft state,
requested-reviewer state, required checks, and latest bot findings. Re-evaluate
older human findings against the current head; do not reuse a stale review
without checking the intervening commits.

## Writing standard

- Keep commit subjects short and factual. If a body is needed, use short factual lines directly tied to the changed code, tests, or review.
- Keep PR descriptions and review comments concise, factual, and scoped to the current change. Do not include storytelling, chat transcripts, roadmaps, future plans, or unrelated context.
- Put code-specific feedback in inline comments. Use PR-level comments only for concise summaries, checks, or dispositions.
