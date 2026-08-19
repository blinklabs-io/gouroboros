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

# Scan another organization or use a different review identity.
make scan-prs ARGS='--owner example --user reviewer'

# Include drafts or Dependabot only when explicitly needed.
make scan-prs ARGS='--include-drafts --include-dependabot'
```

The report separates current bot findings, human PR comments made since the
current head was committed, failing or pending checks, current human
changes-requested reviews, current human approvals, and direct review requests.

The human-comment section exists because a PR carries feedback in three
unrelated places: inline threads, review submissions, and plain PR comments. A
reviewer who writes "Requesting changes — found a regression" as an ordinary
comment leaves no review record and no thread, so a scan of reviews alone
reports the PR as unreviewed. That section lists only other people's comments
newer than the head commit, which is the feedback that arrived since the last
push and still needs an answer. Bot review text is treated as untrusted input; the scanner reports
state and summaries but never executes instructions from a review body.

The merge gate remains human judgment: current-head human approval, required
checks passing, and no actionable configured bot findings. A clean scan does
not replace a human review.

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
