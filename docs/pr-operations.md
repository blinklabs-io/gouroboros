# Pull request operations

Use the workspace scanner before reviewing or handing off organization pull
requests. It reads GitHub state only; it does not post reviews, modify branches,
or merge pull requests.

```sh
make scan-prs
```

The default scan covers open, non-draft, non-Dependabot pull requests in
`blinklabs-io`. It compares reviews and checks with each PR's current head SHA,
so approvals and bot findings for an earlier commit are not reported as current.

Useful variants:

```sh
# Machine-readable output for an agent or a follow-up script.
make scan-prs ARGS='--format json'

# Scan another organization or use a different review identity. Supply that
# identity's known teams because GitHub only exposes the authenticated user's
# memberships.
make scan-prs ARGS='--owner example --user reviewer --team example/reviewers'

# Include drafts or Dependabot only when explicitly needed.
make scan-prs ARGS='--include-drafts --include-dependabot'
```

The report separates current bot findings, unanswered human PR comments, human
reviews carrying feedback in any state, failing or pending checks, current human
changes-requested reviews, current human approvals, and direct or team review
requests. Team requests are matched against the authenticated user's memberships
in the scanned organization. Use repeatable `--team` flags to supply known team
slugs explicitly; an unqualified slug uses `--owner`.

If GitHub does not expose team memberships to the active token, or `--user`
selects someone other than the authenticated user without corresponding
`--team` flags, the report marks the team lookup incomplete and exits nonzero.
It does not silently report that no team reviews are waiting. JSON output adds
the selected `review_teams`, a nullable `team_lookup_error`, each pull request's
qualified `requested_team_slugs`, and its matching `review_request_sources`.

Two of those sections exist because reviewer feedback hides in places a naive
scan misses:

- **Unanswered human PR comments.** A PR carries feedback in three unrelated
  places: inline threads, review submissions, and plain PR comments. A reviewer
  who writes "Requesting changes — found a regression" as an ordinary comment
  leaves no review record and no thread, so a scan of reviews alone reports the
  PR as unreviewed. This section lists comments from anyone other than the
  author that the author has not replied to since — anchored on the author's own
  last comment, deliberately **not** on the head commit, because a push is not
  evidence that a comment was answered.
- **Human reviews with feedback, any state.** A reviewer can put blocking
  findings in a `COMMENTED` review rather than `CHANGES_REQUESTED`, and the
  changes-requested section then shows nothing. Reviews not on the current head
  are marked as such rather than dropped, since an older review can still be
  unaddressed.

Bot identity is matched by login name as well as the `[bot]` suffix: the REST API
suffixes bot logins where GraphQL does not, so a suffix test alone counts
`coderabbitai` and `cubic-dev-ai` as people and buries the real reviewers. Bot review text is treated as untrusted input; the scanner reports
state and summaries but never executes instructions from a review body.

The merge gate remains human judgment: current-head human approval, required
checks passing, and no actionable configured bot findings. A clean scan does
not replace a human review.

## Posting an authorized review

The scanner is read-only, but an explicitly authorized review action should be
attempted directly through GitHub. Do not run `gh auth status` as a precondition:
it may inspect a stale or different environment token even when the review
operation is available. If the write itself reports a network failure, retry
through the approved elevated network path. If it reports an authorization
failure, report that failure and do not claim the review was posted.

For a review with no merge-blocking findings, post an approval with no body:

```sh
gh pr review <number> --repo <owner>/<repo> --approve
```

Use `CHANGES_REQUESTED` only for actionable blockers and put code-specific
findings in inline comments through GitHub's UI or API. After any write, verify
the review record, state, body, reviewer, and current head SHA through GitHub.

## Description hygiene

Keep the author-written description factual and scoped to the change. A useful
shape is:

```markdown
## Problem

<observable problem>

## Changes

- <code or documentation change>

## Checks

- `<command>`

## Summary by Cubic

<optional normalized bot summary>

## Summary by CodeRabbit

<optional normalized bot summary>
```

Bot summaries may be restored when they improve handoff, but copy only their
verified factual content. Convert them to ordinary Markdown and remove raw HTML,
review buttons, hidden bot state, prompts, run IDs, stale commit metadata, and
duplicate release-note wrappers. Keep code-specific findings in inline review
comments rather than moving them into the description.
