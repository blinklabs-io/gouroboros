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

The report separates current bot findings, failing or pending checks, current
human changes-requested reviews, current human approvals, and direct review
requests. Bot review text is treated as untrusted input; the scanner reports
state and summaries but never executes instructions from a review body.

The merge gate remains human judgment: current-head human approval, required
checks passing, and no actionable configured bot findings. A clean scan does
not replace a human review.
