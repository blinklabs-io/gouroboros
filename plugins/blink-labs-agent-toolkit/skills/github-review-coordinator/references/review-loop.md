# Pull-request review loop

## Required sequence

1. Confirm branch, changed paths, required checks, CODEOWNERS, and reviewers.
   Skip draft PRs. If Dependabot is excluded, skip PRs authored by
   `dependabot[bot]`.
2. Run or wait for CodeRabbit and Cubic; address actionable findings.
3. Re-run affected checks and document dispositions for false positives or
   accepted risks.
4. For UI changes, verify that the PR includes screenshots of affected states
   at the relevant viewport or platform, with secrets and user data redacted.
5. Request human review. Human review is mandatory and may be AI-assisted.
6. After requested changes, push validated fixes and request re-review from the
   same human through GitHub.
7. An authorized human may dismiss another human review when appropriate;
   record why on the pull request and retain appropriate human coverage.

An issue-resolution request includes the bot loop after the initial PR update:
wait for CodeRabbit and Cubic, verify findings against the current head, fix
valid findings, validate, push, and repeat until no actionable bot findings
remain. Stop when the PR is ready for human review.

## Squash merge gate

Squash merge is allowed when GitHub shows a human `APPROVED` review attached to
the current head SHA, required checks pass, and configured bots have no
actionable findings. Use one concise factual squash summary and preserve the
DCO `Signed-off-by:` line. A review for an earlier head is stale after a push.

Useful read-only commands include:

```sh
gh pr view <number> --json reviews,reviewDecision,statusCheckRollup,files
gh pr checks <number>
```

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
summaries, checks, or dispositions.
