---
name: github-review-coordinator
description: Discover Blink Labs pull-request review work, including direct and team requests, and coordinate CodeRabbit, Cubic, and required human reviews. Use when checking for reviews or preparing, updating, or handing off a GitHub pull request.
---

# GitHub Review Coordinator

Use this skill for pull-request review workflow, not for deciding whether a
code finding is technically correct. Read the [review loop reference](references/review-loop.md)
and the target repository's contribution guidance.

## Review sequence

1. When discovering review work, include pull requests assigned directly to the
   authenticated account and pull requests assigned to any of its teams. If
   team memberships cannot be queried, report the search as incomplete rather
   than claiming no review requests exist. Then confirm the PR, base branch,
   changed repositories, checks, CODEOWNERS, and required reviewers. Exclude
   draft PRs. If the user excludes Dependabot, exclude PRs authored by
   `dependabot[bot]`.
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

When the user authorizes a review action, attempt the requested GitHub write
directly. Do not make `gh auth status`, a stale `GITHUB_TOKEN`, or another
preflight diagnostic a gate: the diagnostic may inspect a different credential
or transport from the operation that is actually available. If the write itself
fails because of network connectivity, retry through the approved elevated
network path. If it fails authorization, report that actual failure and do not
claim the review was posted.

Choose the review disposition from the findings. Use an approval-only review
with an empty body when there are no merge-blocking findings; do not turn
trivial recommendations into `CHANGES_REQUESTED`. Use `CHANGES_REQUESTED`
only for actionable blockers and put those findings in inline comments through
GitHub's UI or API. After every write, verify the review record, reviewer, state,
empty or non-empty body as intended, and current head SHA through GitHub.

When the user asks to resolve an issue, iterate after the initial PR update:
wait for CodeRabbit and Cubic, or document CodeRabbit rate limiting and use
Cubic alone, then verify each finding against the current head, fix valid
findings, validate, push, and repeat until the available bots have no
actionable findings. Stop at human-review readiness.

## Do not push what you have not verified

CI minutes and bot review passes are shared, and a push spends both. Pushing to
find out whether a change works offloads your uncertainty onto the pipeline and
onto whoever reads the resulting bot findings, so the local check comes first —
even when it is slower than a CI run, and especially when someone is waiting.

Verify the specific thing the change claims, with the tool that decides it:

- A lint fix is not done when the directive is written, it is done when the
  linter agrees. `//nolint` binds to the line the diagnostic is *reported* on,
  which for a multi-line call or a `k := string(key)` conversion is often not
  the line a reader would pick. One scoped run settles it:
  `golangci-lint run --default=none -E <linter> ./pkg/...`.
- Match the tool version CI uses. A finding that only exists under the newer
  release cannot be reproduced or cleared with the older binary, and a formatter
  bundled in the linter is not the one on your PATH.
- Scope the run to the packages you touched. A whole-tree run from a cold cache
  can take an order of magnitude longer than CI's warm one, which is what
  tempts the premature push in the first place.
- State the gap when one remains. "Verified on these packages, whole-tree
  confirmation comes from CI" is honest; pushing and calling it verified is not.

If a check is genuinely too slow to complete locally, say so and let the user
decide whether to spend the CI cycle — that is their call, not a default.

## Never rewrite pushed history without being asked

Do not rebase, amend, or force-push a branch that has already been pushed —
not on anyone else's PR, not on our own, and not when the rewrite is provably
identical in content and is the obvious way to clear a check.

A task-level authorization does not cover it. "Do a PR sweep", "act on the clear
ones", or "fix the failing checks" is not permission to rewrite published
history; that needs approval for that branch, at that time, and approval on one
branch never carries to the next.

`--force-with-lease` is not a safety net for this. It only proves nobody else
pushed in the meantime, so it succeeds in exactly the case that upsets people.
The guard is judgment, not tooling.

The tempting case is a stale check, so know why re-running does not help:
re-running an old workflow run replays that run's **original** merge ref, so it
never picks up a new base. A fix that landed on the base branch afterwards will
not appear, and the check keeps failing on code that is already fixed. That is a
reason to explain the situation and let the branch owner decide — not a licence
to rebase. State plainly that the branch needs a fresh push or a merge-queue run,
and stop there.

## A failing check the diff cannot explain

When a check fails on a PR whose diff could not plausibly cause it —
`govulncheck` red on a `.gitignore`-only change, `gofumpt` red on a
workflow-only change — do not start fixing the PR. Reproduce the check on
`origin/main` first.

Both cases in one sweep were drift on the base branch, not regressions: an exact
`go-version` pin went stale as advisories landed, and an unpinned
`golangci-lint-action` picked up a stricter bundled formatter. The flagged code
had been untouched for months. "Fixing the PR" would have fixed nothing.

The sequence that works:

1. **Read the version CI actually resolved**, from the job log — `Installing
   golangci-lint binary v2.13.0` — not the version on your PATH. A formatter
   bundled in a linter is not the standalone binary, and a finding that only
   exists under the newer release cannot be cleared with the older one.
2. **Run the same check on `origin/main`.** If main fails too, the fix is its own
   PR against main that unblocks every open PR, not a commit buried in whichever
   branch happened to surface it.
3. **Check whether a sibling open PR already fixes it** before writing your own.
   Grep the open PRs' diffs for the file and line. One sweep opened a redundant
   PR for a formatting fix another open PR already carried in its second commit;
   merging that PR would have unblocked the same branch with no new work.
4. **Audit the class across sibling repositories** before reporting. Toolchain
   drift is never one repository. After fixing the first two, the same unpinned
   action was found latent in fourteen — visible only because their last green
   run predated the release.
5. **File one issue for the organization-wide part**, rather than a one-off
   change to whichever file was touched. Confirm at least one repository with the
   real linter before asserting the list; enumerate the rest with the standalone
   formatter and say which method produced which number.

Repository exclusions decide what CI reports, so read them before counting:
`run.tests: false` hides every `_test.go` finding, and
`exclusions.generated: lax` hides generated trees. A raw formatter run over the
whole repository will overcount against what `lint` actually fails on.

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

- **Read the contract of everything you call, wrap, or cite as precedent.** Do
  this first; it is the step that pays. Three consecutive rounds on one PR found
  a defect already written down in the neighbouring code: that callers add the
  component prefix, that the sibling path clears its state when exhausted, that
  the function releases its mutex around the network request. None of it needed
  inferring. Look in all three places, because obligations live in all three: the
  doc comment above the declaration, the leading comment inside the body, and —
  for a thin wrapper — the function it delegates to. And when you cite a
  precedent, read the whole of it: copying the half that sets a field and missing
  the half that clears it is its own review round. The `callee-contract-notice`
  hook reprints these sentences at commit time, but the hook only reads Go doc and
  body comments one delegation deep; the reading is still yours.
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

Squash merge is allowed only when **all four** hold:

1. The PR's author is us. Read `.user.login` (REST) or `.author.login` (GraphQL)
   for that specific PR and compare it to the authenticated account. Do not infer
   it from the branch name, from having worked on the change, or from having
   opened a sibling PR in the same sweep.
2. GitHub shows a human `APPROVED` review for the current head SHA. An approval
   for an earlier head is stale after a push.
3. Required checks pass.
4. Configured bots have no actionable findings.

Use one concise factual squash summary and preserve the DCO `Signed-off-by:`
line.

"Approved and green" is not the gate; "ours, approved, and green" is. The first
three conditions are easy to verify and the ownership one is easy to skip, so
verify it last and explicitly, immediately before the merge call.

A general statement of merge policy does not convert someone else's PR into
ours. "Anyone here can merge an approved PR", or a reviewer confirming that we
are the author of the PR under discussion, resolves per PR against condition 1 —
it never widens the set. When a broad-sounding grant would have you merge a PR
someone else authored, confirm that specific PR before pressing merge; the cost
of asking is one message, and the merge cannot be un-pressed.

Beware the tooling trap in condition 2: `gh api --jq` does **not** accept `--arg`.
`gh api ... --jq --arg h "$sha" '...'` fails with `accepts 1 arg(s), received 4`,
and inside a loop that error is easy to swallow — every PR then reports no
approval on head, or, with inverted logic, every PR reports one. Pipe the JSON to
a separate `jq --arg` invocation and check that at least one row came back
non-empty before trusting a batch verdict. See the
[review loop reference](references/review-loop.md).

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
