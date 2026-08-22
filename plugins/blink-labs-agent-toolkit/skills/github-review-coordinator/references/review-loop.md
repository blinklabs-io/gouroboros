# Pull-request review loop

## Required sequence

1. Discover pull requests requested directly from the authenticated account and
   from any of its teams. If team memberships cannot be queried, report the
   search as incomplete rather than claiming there are no review requests.
   Confirm branch, changed paths, required checks, CODEOWNERS, and reviewers,
   each read against the current head SHA and reported with it. Prefer
   `scripts/scan-prs.py`, which already anchors reviews and check runs to
   that head, over an ad-hoc query.
   Skip draft PRs. If Dependabot is excluded, skip PRs authored by
   `dependabot[bot]`.
2. Run or wait for CodeRabbit and Cubic; address actionable findings. If
   CodeRabbit is rate-limited, document it and use a completed Cubic review as
   sufficient bot review. Cubic's monthly allowance also runs out, so check the
   review record for which bots reviewed the current head instead of reading an
   empty thread list as a clean pass, and record the local review standing in
   for any that did not run.
3. Re-run affected checks and document dispositions for false positives or
   accepted risks.
4. For UI changes, verify that the PR includes screenshots of affected states
   at the relevant viewport or platform, with secrets and user data redacted.
5. Request human review. Human review is mandatory and may be AI-assisted.
6. After requested changes, push validated fixes and request re-review from the
   same human through GitHub.
7. An authorized human may dismiss another human review when appropriate;
   record why on the pull request and retain appropriate human coverage.

## Posting the review

When the user authorizes a review write, try the requested operation before
diagnosing credentials. `gh auth status` can report an invalid environment token
even when `gh pr review` can use another available credential. A failed write is
the signal to diagnose; if the failure is network connectivity, retry with the
approved elevated network path. Do not claim success until GitHub shows the
review.

For a clean review with only non-blocking recommendations, submit an empty-body
approval:

```sh
gh pr review <number> --repo <owner>/<repo> --approve
```

For actionable blockers, request changes and attach each code-specific finding
inline through GitHub's UI or API. Keep the review body empty when approving;
do not turn a trivial test or accessibility recommendation into a blocking
changes-requested review.

After posting, verify the review list against the current head SHA and confirm
the reviewer login, state, and body. If the authenticated `gh` read path cannot
connect, use a read-only GitHub API request through the approved network path
for verification.

An issue-resolution request includes the bot loop after the initial PR update:
wait for CodeRabbit and Cubic, or document CodeRabbit rate limiting and use
Cubic alone, then verify findings against the current head, fix valid findings,
validate, push, and repeat until the available bots have no actionable findings.
Batch verified fixes into one push: each push spends a metered review pass, and
neither bot may be available to spend.
Stop when the PR is ready for human review.

## Squash merge gate

Only the author merges their own pull request. Whoever presses merge takes
responsibility for the code, so approving a change is not the same as owning
it — hand an approved PR back to its author rather than merging it for them.
The sole exception is `dependabot[bot]`, which cannot merge its own PRs.

Squash merge is allowed when the PR's author is us, GitHub shows a human
`APPROVED` review attached to the current head SHA, required checks pass, and
configured bots have no actionable findings. Use one concise factual squash
summary and preserve the DCO `Signed-off-by:` line. A review for an earlier head
is stale after a push.

Check authorship per PR, from the PR record, immediately before merging. Do not
infer it from the branch name, from having authored the change, or from a general
statement that approved PRs may be merged.

Useful read-only commands include:

```sh
gh pr view <number> --json reviews,reviewDecision,statusCheckRollup,files
gh pr checks <number>
gh pr view <number> --json author,headRefOid,mergeStateStatus
```

`gh api --jq` does not accept `--arg`, so the obvious one-liner for "is the
approval on the current head" fails with `accepts 1 arg(s), received 4`. In a
loop that error is swallowed and every PR reports the same wrong answer. Pipe to
a separate `jq`:

```sh
head=$(gh api "repos/$OWNER/$REPO/pulls/$N" --jq '.head.sha')
gh api "repos/$OWNER/$REPO/pulls/$N/reviews" > /tmp/reviews.json
jq -r --arg h "$head" '
  [.[] | select(.state == "APPROVED" and .commit_id == $h) | .user.login]
  | unique | join(",")' /tmp/reviews.json
```

Filter bots by login **name**, not the `[bot]` suffix — REST returns
`coderabbitai[bot]` where GraphQL returns `coderabbitai`. When a batch check
reports the same verdict for every PR, treat that as a probable query bug and
verify one PR by hand before acting on the batch.

Note that `gh pr edit` and any `--json` field naming a login or team can fail on
a token without `read:org`. Fall back to
`gh api -X PATCH repos/OWNER/REPO/pulls/N --input body.json` for edits, and to
plain REST reads for reviewer state.

### Always `--paginate` when verifying a write

`gh api` list endpoints return **30 items per page** by default and give no hint
that more exist. On a PR with an active bot, 30 comments is a single review's
worth, so a verification read of `pulls/N/comments` can show none of your own
and look exactly like a silent failure.

That misreading is expensive: it invites re-posting comments that already landed.
A submitted review's inline comments are attached to it and cannot be un-posted,
so the duplicates have to be deleted one by one afterwards
(`gh api -X DELETE repos/OWNER/REPO/pulls/comments/ID`).

Two habits avoid it:

- Pass `--paginate` on every verification read, and prefer the identifiers the
  write already returned. A `POST .../reviews` response carries the review `id`;
  filter on `.pull_request_review_id == <id>` rather than re-deriving the set.
- Treat "my write is missing" as a query bug until proven otherwise. Before
  re-posting anything, confirm with a second query shaped differently — by id,
  paginated, or unfiltered with a count. A write that returned a non-error
  response with an id almost certainly succeeded.

Use the GitHub UI or API for reviewer requests and dismissals. Verify the PR
state afterward; never infer that a request, approval, or dismissal happened
from a local commit or comment.

Before posting, verify the current head SHA and re-check older findings against
the current diff. A changed head requires a fresh review decision.

## Writing standard

Keep commit messages, PR descriptions, and review comments short, factual, and
scoped to the changed code, tests, and review state. Do not include storytelling,
chat transcripts, roadmaps, future plans, or unrelated context. Put
code-specific feedback in inline comments; use PR-level text only for concise
summaries, checks, or dispositions. Useful Cubic and CodeRabbit summaries may
remain in a PR description when they are converted to plain Markdown with clear
attribution. Remove generated HTML, buttons, hidden state, prompts, run IDs, and
stale commit metadata.
