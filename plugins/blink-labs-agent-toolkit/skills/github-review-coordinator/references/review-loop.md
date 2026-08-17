# Pull-request review loop

## Required sequence

1. Confirm branch, changed paths, required checks, CODEOWNERS, and reviewers.
2. Run or wait for CodeRabbit and Cubic; address actionable findings.
3. Re-run affected checks and document dispositions for false positives or
   accepted risks.
4. Request human review. Human review is mandatory and may be AI-assisted.
5. After requested changes, push validated fixes and request re-review from the
   same human through GitHub.
6. An authorized human may dismiss another human review when appropriate;
   record why on the pull request and retain appropriate human coverage.

Useful read-only commands include:

```sh
gh pr view <number> --json reviews,reviewDecision,statusCheckRollup,files
gh pr checks <number>
```

Use the GitHub UI or API for reviewer requests and dismissals. Verify the PR
state afterward; never infer that a request, approval, or dismissal happened
from a local commit or comment.
