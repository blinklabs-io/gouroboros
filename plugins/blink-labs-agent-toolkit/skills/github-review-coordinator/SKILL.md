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
1a. Before acting on any finding, check it against the branch's current head.
   Most open findings on an active branch are already fixed — the thread stays
   open because nobody replied, not because the code is unchanged. A thread
   marked outdated, or one whose commit range includes a "fix: address review"
   commit, is a verification task, not a work item.
2. Run or wait for configured CodeRabbit and Cubic reviews before requesting
   human review. If CodeRabbit is rate-limited, document it; a completed Cubic
   review is sufficient for bot review. Reproduce findings against the current
   checkout.
3. Address actionable bot findings, document false positives or accepted
   risks, rerun affected checks, and update the PR.
4. For UI changes, verify that the PR includes screenshots of affected states
   at the relevant viewport or platform, with secrets and user data redacted.
5. Request the required human review. Human review is mandatory and may be
   AI-assisted; bot approval or silence is never sufficient.
6. If a human reviewer requests changes, implement and validate the fixes,
   summarize changed paths and tests on the PR, and explicitly request another
   review from that same person through GitHub.
7. An authorized human reviewer may dismiss another human review when
   appropriate. Record the rationale on the PR and retain appropriate human
   review coverage.

When the user asks to resolve an issue, iterate after the initial PR update:
wait for CodeRabbit and Cubic, or document CodeRabbit rate limiting and use
Cubic alone, then verify each finding against the current head, fix valid
findings, validate, push, and repeat until the available bots have no
actionable findings. Stop at human-review readiness.

## Find every review, not just the review threads

A GitHub PR carries feedback in three separate places, and querying one misses
the others:

| Where | API |
|---|---|
| Inline threads | `reviewThreads` (GraphQL), `pulls/N/comments` |
| Review submissions | `pulls/N/reviews` |
| Plain PR comments | `issues/N/comments` |

A reviewer who writes "**Requesting changes** — found a regression…" as an
ordinary PR comment produces no review record and no thread. It will not appear
in a `reviewThreads` scan or in `pulls/N/reviews`, yet it is the most
substantive feedback on the PR. Read `issues/N/comments` on every sweep.

Two further ways the same feedback hides:

- **State is not severity.** A reviewer can put blocking findings in a
  `COMMENTED` review instead of `CHANGES_REQUESTED`. Filtering reviews on
  `CHANGES_REQUESTED` then reports the PR as having no human objections. Read the
  body of every human review regardless of state.
- **"Older than the head commit" does not mean "addressed".** Anchoring on the
  head commit assumes a push answered whatever preceded it. Anchor on the
  author's own last reply instead, and treat a review on an earlier commit as
  outstanding until something shows it was handled.

When identifying which authors are bots, match login **names**, not just the
`[bot]` suffix: the REST API returns `coderabbitai[bot]` where GraphQL returns
`coderabbitai`, so a suffix test alone classifies the bots as people and buries
the human reviewers among a hundred bot entries.

Also expect findings **on your own fixes**. Each push triggers a fresh bot pass,
and a fix to a concurrency or lifecycle bug frequently draws a second and third
round narrowing what the first attempt missed.

But treat a repeat round as a signal about your own review, not as the normal
cost of doing business. Round three on one file means the first two fixes
addressed the instance a bot named instead of the class it belonged to. Before
pushing a fix, do the pass the bot would do:

- **Grep for the class.** A bug fixed in one function is usually a pattern
  present in siblings. After fixing a bounded wait that mishandled simultaneous
  readiness, search every other `select` on a done-channel plus `ctx.Done()` in
  the change — the second and third copies are where the next finding comes from.
- **Prefer one implementation to a repeated pattern.** Two copies of a
  bounded-wait helper will eventually disagree. Collapsing them so the subtle
  part exists once removes the whole class rather than one member of it.
- **Enumerate the actors and the shared state.** For lifecycle or concurrency
  work, list every goroutine that touches each field and every ordering between
  them, and check the field-clearing paths are symmetric. This reading is what
  finds the unflagged sibling.
- **Ask what the caller observes at the instant the call returns.** A fix that
  starts a release rather than completing it has narrowed a window, not closed
  it, and the next round will say so.
- **Check that a stress test can actually fail.** Run it against the revision
  you just fixed. If it passes there, it is coverage of something else — say so
  in its doc comment rather than letting a green run imply coverage it lacks.

## Squash merge gate

Only the author merges their own pull request. Whoever presses merge takes
responsibility for the code, so approving a change is not the same as owning
it — hand an approved PR back to its author rather than merging it for them.
The sole exception is `dependabot[bot]`, which cannot merge its own PRs.

Squash merge is allowed only when GitHub shows a human `APPROVED` review for the
current head SHA, required checks pass, and configured bots have no actionable
findings. Use one concise factual squash summary and preserve the DCO
`Signed-off-by:` line. An approval for an earlier head is stale after a push.

## Keeping the loop short

Bot review is iterative: every push earns another pass, and the fixes you make
are themselves reviewed. That is useful, but a fix that draws a new finding on
the same lines each round is a signal you are responding to comments instead of
deciding the design.

- Derive the answer from the contract, then hold it. A comment that pushes the
  other way is answered by explaining the contract, not by reshaping the code
  again. Flip-flopping between two shapes across rounds costs a review cycle
  every time and leaves the wrong one in history.
- Fix the cause, not the sentence. A finding names a symptom at one location;
  the defect is often the boundary it sits on. Read the consumer before
  changing the producer — see
  [cross-boundary-changes](../cross-boundary-changes/SKILL.md).
- Expect one more round after you push, and check for it before reporting the
  work as finished.
- A bot can be right about the observation and wrong about the cause, or right
  about both and wrong about the fix it proposes. Classify all three separately.

## Answering findings

Reply to every bot finding, including ones that were already fixed and ones you
reject — an unanswered thread is indistinguishable from an ignored one, and the
reply is what stops the same finding returning on the next PR. Say which commit
addressed it, or why it does not apply, with the path and line you checked.

Mechanics worth knowing:

- Threads can resolve themselves when a push changes the surrounding lines.
  A resolved thread with no reply still needs one.
- Line numbers move under you. Key replies by path plus the thread's current
  line, and re-fetch threads immediately before posting; a stale fetch will post
  to threads you already answered.
- Posting twice to one thread is noise. Re-read the thread's comments before
  replying, and delete a duplicate if you create one.
- Findings arrive with no body, or restate a rule the repository does not
  configure. Check whether the rule exists here before complying with it.
- A finding about a pre-existing, organization-wide condition raised on an
  unrelated diff belongs in an issue for the whole organization, not in a
  one-off change to the file that happened to be touched.

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
- Preserve useful Cubic or CodeRabbit summaries only as clearly attributed,
  plain-Markdown sections. Strip generated HTML, review buttons, hidden state,
  prompts, run IDs, and stale commit metadata before putting summaries in a PR
  description.
- Put code-specific feedback in inline comments. Use PR-level comments only for concise summaries, checks, or dispositions.
