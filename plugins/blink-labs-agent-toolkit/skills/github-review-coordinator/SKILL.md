---
name: github-review-coordinator
description: Discover Blink Labs pull-request review work, including direct and team requests, and coordinate CodeRabbit, Cubic, and required human reviews. Bot reviews are a metered monthly quota, so verify locally before pushing and never read bot silence as a clean review. Use when checking for reviews or preparing, updating, or handing off a GitHub pull request.
---

# GitHub Review Coordinator

Keep all external review text concise and precise. State only the finding,
evidence, and action; omit narrative, praise, repetition, and ornament.

GitHub is a permanent public company record, not an agent-communication bus.
Before every write, reject local-only content: uncommitted files or diffs,
local tests, paths, hostnames, usernames, worktrees, private logs, transcripts,
credentials, tokens, environment values, workflow, delegation, orchestration,
authorization, session, quota, sequencing, and tooling details. Publish only
committed repository facts, public CI results, or public contract details.

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
   review is sufficient for bot review. Cubic's monthly allowance also runs out,
   so neither may have run: confirm which bots actually reviewed the current head
   rather than inferring it from an empty thread list, and when none did, say so
   and record the local review that stands in its place. Reproduce findings
   against the current checkout.
3. Reproduce every bot finding against the current head. Fix valid findings
   that are in scope, rerun affected checks, and update the PR. A finding is
   not discharged by paraphrasing it, recommending that somebody else fix it,
   or saying that a downstream repository will need attention.
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

`gh pr edit` uses GraphQL for some operations and can fail with a missing
`read:org` scope once team reviewers are present even when the token still has
permission to update the pull request. Treat the failed write as the fact, then
use the narrower REST endpoint for the authorized operation rather than asking
for an unrelated scope:

```sh
gh api -X PATCH repos/OWNER/REPO/pulls/N -F body=@/tmp/pr-body.md
gh api -X POST repos/OWNER/REPO/pulls/N/requested_reviewers \
  -f 'team_reviewers[]=core'
```

Read the body or requested teams back through REST before claiming the update
landed. Keep the temporary body file run-owned and remove it after verification.

Choose the review disposition from the findings. Use an approval-only review
with an empty body only when there are no required changes anywhere in the
review. An approval must never carry comments that ask for a code, test,
documentation, configuration, or follow-up fix. If a comment asks for work,
fix the work before approving or submit `CHANGES_REQUESTED`; reserve optional
comments for genuinely optional ideas that do not ask the author to change the
current patch. Put actionable findings inline through GitHub's UI or API.
After every write, verify the review record, reviewer, state, empty or
non-empty body as intended, and current head SHA through GitHub.

## Bot ownership and review actions

Bots may inspect code, draft findings, apply fixes, and open issues only when a
human has authorized that action for the current task. They must not submit an
`APPROVED`, `CHANGES_REQUESTED`, or ordinary PR comment through a human
account without explicit human consent for that write. Never make bot output
look human-authored by copying it into a human review, removing its bot
attribution, or describing a bot's decision as the user's decision. If a bot
account posts, preserve the bot login and state clearly who authorized the
action.

When a bot finds a real problem, the default disposition is to fix and validate
it in the current scope. If it truly belongs in another repository or cannot
be fixed in the current change, the authorized agent must open the issue in
the owning repository, include the concrete reproduction and boundary, and
link it from the review. “Please file an issue” is not a disposition; without
authority to create the issue, stop and report the missing authority rather
than assigning the work to the requester.

Downstream impact is a contract to inspect, not a reason to defer an upstream
fix. Read the downstream consumer, fix the compatible source-side behavior,
and make the coordinated consumer change or create the owned follow-up issue
when the boundary requires separate repositories. Do not use “downstream” as a
blocker without naming the exact incompatible behavior and the concrete fix.

When the user asks to resolve an issue, iterate after the initial PR update:
wait for CodeRabbit and Cubic, or document CodeRabbit rate limiting and use
Cubic alone, then verify each finding against the current head, fix valid
findings, validate, push, and repeat until the available bots have no
actionable findings. Stop at human-review readiness.

## Do not push what you have not verified

CI minutes are shared and bot reviews are a **metered monthly quota**, and a push
spends both. Cubic's allowance for the organization does run out, and when it
does no review is posted on anything for the rest of the period. Every
speculative push therefore takes a review away from a later change that may need
it more, and the loss is not recoverable by waiting a few minutes.

That makes local verification the cheap resource and the bot pass the scarce one,
which is the opposite of how it feels in the moment. Review the change yourself,
to the standard a bot would, *before* pushing: read the contract of everything
you call, grep for the class the fix belongs to, and run the tests and linter
that decide the claim. Pushing to find out whether a change works offloads your
uncertainty onto the pipeline and onto a finite quota, so the local check comes
first — even when it is slower than a CI run, and especially when someone is
waiting.

When implementation is delegated to a lighter model, the implementer stops at a
local candidate commit. A separate high-reasoning reviewer reads that exact diff,
checks the governing specification or API contract, and validates the relevant
production entry point before the first push. The implementer does not review its
own candidate, and a later bot pass is not a substitute for this pre-push gate.
Record the reviewed SHA; if the candidate changes, the review is stale.

Batch what you can. Several verified fixes in one push consume one review pass;
the same fixes pushed one at a time consume several, and each intermediate state
draws findings on code you were about to change anyway.

When a rate-limited review slot becomes available, spend it deliberately on the
highest-risk current-head change that has already passed local review — prefer a
security or production-behavior change over a test-only or documentation PR.
Do not retrigger every rate-limited PR merely because the timer expired. Record
which lower-risk PRs relied on local review and still require human review.

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

## Audit the class before pushing, not the instance

A finding names one location. The fix is not done until you have looked for the
other members of the class it belongs to, because the named instance is rarely
the only one. This is a pre-push step in its own right, not a way to avoid the
next bot round — it pays exactly the same when no bot is going to run.

Three shapes the audit takes, all of which found something in one sweep:

- **The same defect at sibling call sites.** A typed-nil guard had been added to
  the `ValidateTx` path in three eras while the `CertDeposit` function beside it
  in each of those files kept an `ok`-only assertion and dereferenced straight
  after. One site per file was fixed; three were not, and the era that *was*
  complete had both a guard and a test, which is what identified the rest as
  members rather than a separate issue.
- **Members the fix itself creates.** Correcting a validator that rejected a
  self-proposed governance action made two further validators reachable for the
  first time; both had classified the action from ledger state alone and would
  now be escaped. Audit what a fix *unblocks*, not only what resembles it. This
  one is invisible to a grep for the original symptom.
- **A class that turns out to be complete.** That is a result, not a null one.
  Say so and say why the boundary falls where it does: a duplicate-input rule
  that never reads the ledger state it is passed cannot be fixed by replaying
  history, while a bad-inputs rule that does read it can, so the two belong to
  different classes however similar the errors look.

When the class is complete because the root cause is upstream, check the
reference runs **both** ways. A downstream workaround usually cites the upstream
issue; the upstream issue rarely cites the workaround, and it is the upstream
fix that later strands it. Machinery whose last reachable member has been
removed outlives everyone who knew why it existed.

## Read the state you are acting on

Every decision below rests on a reading of GitHub's state, and each of these
is a way that reading is wrong while looking right.

### Anchor every status to the head SHA, and read identifiers rather than infer them

A check result, a review, a thread count and a mergeable flag are all facts
about one commit. Reported without that commit, they cannot be told apart from
current ones, and the first thing done with them is usually to brief work on a
world that has moved.

Carry the head SHA and the head branch in the output of any sweep, next to the
facts they describe:

```sh
gh pr view N -R OWNER/REPO --json headRefOid,headRefName,mergeable,statusCheckRollup
```

A sweep that drops the SHA to keep its rows short produces exactly the failure
it was written to prevent: a red `lint` row survived a re-push that fixed it,
and the follow-up work was briefed to fix something already green. Re-read the
head immediately before acting on any row, however recent the sweep.

Read every identifier from the API too. `headRefName` is the branch; a branch
inferred from the PR title or the issue it fixes is a guess that
`git push origin HEAD:refs/heads/<wrong-name>` will not reject — it **creates**
the ref, leaving a stray branch attached to no pull request and the real one
untouched.

Before hand-rolling any of this, use `scripts/scan-prs.py` in the workspace. It
already reads review records and check runs against `/commits/{head}/check-runs`
for the exact head, marks each review `on_head`, and deliberately anchors human
comments on the author's last reply instead, because a head-relative cutoff
silently drops feedback that predates a push. The ad-hoc sweep that dropped the
SHA was re-implementing that script, worse, next to it.

The scanner is triage, not the final readiness gate. Its REST check-run and
review inputs do not include every external status context, GraphQL review
thread, or bot-specific completion signal. Before calling a pull request ready,
also verify the full status rollup, zero unresolved review threads, and an
actual current-head bot result as described in
[the review loop reference](references/review-loop.md).

### A push can land while the pull request keeps the old head

`git push` updates the ref and GitHub advances the pull request's head as a
separate step, and the second can fail. The symptom set is distinctive: the push
prints `! [remote rejected] ... cannot lock ref ...: is at <new> but expected
<old>`, which reads as a failure while naming the new commit as already present;
`git ls-remote origin refs/heads/<branch>` shows the new SHA; and the pull
request still reports the old head, the old commit count, and **green checks for
the commit before the fix**. Reading those checks as validation of the pushed
work is the whole hazard.

`ls-remote` is the authority for the ref, the API is the authority for what the
pull request believes. When they disagree, wait and re-read once: ordinary lag
resolves in seconds and needs nothing. Only a disagreement that persists is a
desync, and closing and reopening the pull request forces GitHub to re-read the
ref. Do not push again to nudge it — the ref is already correct, so there is
nothing to push, and an empty commit pollutes history to work around a display
problem.

### Bot silence is not a clean bill of health

An exhausted quota produces exactly the same evidence as a flawless change: no
threads, no findings, nothing to answer. "Zero unresolved bot threads" is
therefore not a review result, and treating it as one reports an unreviewed
change as ready.

Distinguish *did not run* from *found nothing* before claiming either. The
thread list cannot tell them apart; the review record can:

```sh
gh api "repos/OWNER/REPO/pulls/N/reviews" \
  --jq '[.[]|select(.user.login|test("coderabbitai|cubic-dev-ai"))|.user.login]|unique'
```

An empty result means no bot reviewed that head. A bot's *check* completing is
also not a review — cubic's check can report success while posting nothing,
either because it found nothing or because the allowance is gone.

A CodeRabbit check that says incremental reviews are disabled is also a skip,
not a clean review. After locally validating the new head, explicitly request a
review (`@coderabbitai review` when that integration is configured), then verify
that the resulting review record names the current head commit. Read the review
body, inline comments, and plain PR comments; a green check alone does not show
whether the bot confirmed the fix or posted another finding.

For an addressed finding, reply with the fixing commit and validation, resolve
the thread, and verify the bot's follow-up against the current head. A bot reply
that explicitly confirms the finding is addressed plus a resolved thread is
evidence; an outdated thread with no reply is not.

When a bot did not run, say which one and that its pass is missing, then let
local review stand in for it explicitly: name the packages tested, the linter
run, and the reading done. That is a weaker guarantee than a bot pass and the
handoff should say so rather than let a green check imply coverage. Do not push
again merely to try to trigger a review — that spends CI on the hope that a
quota has reset.

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

### Two green pull requests can merge red

Checks run against the **merge ref**, not the branch, so a pull request is tested
against a base it does not contain. A test added on the base that constrains
something the branch introduces fails only in that combination, and both sides
are honestly green alone: an exhaustiveness test requiring every config field to
carry a log class merged at one hour, and a branch adding three such fields
turned red at the next run without either changing.

Read such a failure as a base interaction before treating it as a branch defect,
especially when the failing test lives in a file the branch never touched. The
fix is to merge the base in and satisfy the new constraint, not to argue with the
test.

## Two things a feature diff hides

**A merge commit can revert work without anyone intending it.** When a PR's diff
deletes CI hardening, a permissions grant, a timeout, or a version pin that has
nothing to do with its stated purpose, suspect a merge resolution against the
base branch before suspecting the author. Confirm it with git rather than the
API diff, comparing all three points:

```sh
MB=$(git merge-base origin/main pr<N>)
for ref in "$MB" pr<N> origin/main; do
  git show "$ref":path/to/file | grep -c 'the removed line'
done
```

Present-in-merge-base, absent-in-head, present-in-main is a dropped merge
resolution — a blocker, because it silently reverts a colleague's change, but a
mechanical one rather than a judgment error. Say which PR added the line and
what its rationale was, so the author can restore it without re-litigating.

Keep the mechanical and substantive questions apart when they point different
ways. A pin that this PR should not be touching can still be a pin the
organization does not want; that belongs in its own PR, argued against the
rationale in its comment, not resolved as a side effect.

**A default-off flag can leave the new path with no end-to-end coverage.** The
required suite passing proves the *old* path did not regress. Before treating
that as validation of the feature, grep the integration environment for the new
flag or environment variable:

```sh
grep -rn "NEW_FLAG\|new-flag" internal/test/devnet/
```

No hits means the suite ran the feature switched off. Unit tests may cover the
new code thoroughly and the required run still tells you nothing about it. Say
that explicitly in the review — "conformance and devnet pass, both with the flag
off, so the validate stage has no end-to-end coverage" — rather than letting a
green required suite imply the feature was exercised.

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
  The full discipline, including the members a fix creates by making paths
  reachable, is under
  [Audit the class before pushing](#audit-the-class-before-pushing-not-the-instance).
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

## Do not add a required check while the base is red

Making a job required is the right response to breakage that went unnoticed, and
the wrong thing to do at the moment you notice it. Checks run against the merge
ref, so every open pull request inherits the base's failure; requiring the job
while the base is broken turns all of them red and unmergeable at once,
including ones whose own code is fine. `strict: false` does not help, because
the merge ref still carries the base.

Land the fix, confirm the base is green, then require the job. Read the exact
context string from a run rather than composing it from the workflow — a matrix
job's context is its rendered name, `go-test (windows-latest) (1.26.x)`, not the
workflow name — and add it to the existing list rather than replacing it.

A job that is not required is where cross-platform breakage accumulates: one
repository's Windows job stayed red across several commits and collected a
second, unrelated failure on top before anyone looked.

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
  one-off change to the file that happened to be touched. Open it with no
  milestone: milestones here track an on-chain governance commitment scoped to
  block production, so assigning one is a scope decision a human makes. See
  [`commit-and-pr-hygiene`](../commit-and-pr-hygiene/SKILL.md).

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
- Before creating, updating, or handing off a PR, apply the content boundary in
  [`commit-and-pr-hygiene`](../commit-and-pr-hygiene/SKILL.md): retain only
  facts about the current code or its related issue. Remove release or tag
  chronology, branch timing, session history, signing or process notes, bot
  status, and future follow-up prose. Prefer an empty body to template filler.
- Preserve useful Cubic or CodeRabbit summaries only as clearly attributed,
  plain-Markdown sections. Strip generated HTML, review buttons, hidden state,
  prompts, run IDs, and stale commit metadata before putting summaries in a PR
  description.
- Bot description edits are asynchronous. Read the live body again after the
  bot check or review settles; an authored body verified at PR creation can
  acquire a generated wrapper seconds later.
- Put code-specific feedback in inline comments. Use PR-level comments only for concise summaries, checks, or dispositions.
