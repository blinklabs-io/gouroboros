export const meta = {
  name: 'issue-to-pr',
  description:
    'Implement each Blink Labs issue test-first, then shepherd every resulting commit to a reviewed pull request',
  whenToUse:
    'Working a batch of assigned blinklabs-io issues from triage to pull requests. The developer-to-reviewer handoff is a pipeline stage rather than a reminder, so it cannot be dropped.',
  phases: [
    {
      title: 'Implement',
      detail: 'blink-tdd-developer: a failing test, the smallest fix, a signed local commit',
    },
    {
      title: 'Shepherd',
      detail: 'blink-review-shepherd: pre-publication review, then the pull request and its bot rounds',
    },
  ],
}

const DEV = 'blink-labs-agent-toolkit:blink-tdd-developer'
const SHEP = 'blink-labs-agent-toolkit:blink-review-shepherd'

// Cost is dominated by round trips, not by the size of what each returns: every
// request re-sends the whole accumulated conversation. Measured across earlier
// sweeps, stating these rules in the dispatch prompt cut round trips by 73%.
const EFFICIENCY = `## Efficiency rules (measured; follow them)

Cost is dominated by the number of round trips, not by output size — every
request re-sends the whole accumulated conversation. Batch aggressively:

- Combine independent commands into one call:
  \`go build ./... && go vet ./... && gofmt -l . && golangci-lint run ./...\`
- Read many files in one call:
  \`for f in a.go b.go c.go; do echo "=== $f"; sed -n '1,250p' "$f"; done\`
- Fetch pull-request metadata, diff, review threads, check runs and comments in
  a single \`gh\` call with \`--jq\` field selection.
- Never block waiting on CI. Read the check runs once; if they are still
  pending, record the disposition as withheld and return.

Trim round trips, never rigor. Fail-before reverts and class audits cost little
and are where findings come from.`

function attribution(sessionUrl) {
  const session = sessionUrl ? `\nClaude-Session: ${sessionUrl}` : ''
  return `Every commit uses Conventional Commits and \`git commit -s\` (the OS
prompts for the GPG passphrase). Do not put the issue number in the subject, and
keep the subject at 72 characters or fewer — a workspace guard rejects a longer
one. End each commit message with:

\`\`\`
Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>${session}
\`\`\``
}

// What the developer must hand the shepherd. `committed: false` is a first-class
// outcome, not a failure: an issue whose defect is already fixed upstream has
// nothing to review, and the pipeline must not invent a pull request for it.
const HANDOFF = {
  type: 'object',
  properties: {
    committed: {
      type: 'boolean',
      description: 'true only when a signed local commit exists on a branch',
    },
    reason: {
      type: 'string',
      description:
        'Why there is or is not a commit. When false, the evidence that no change is needed — the merged PR or commit that already fixed it, or what blocked the work.',
    },
    repo: { type: 'string' },
    worktree: { type: 'string' },
    branch: { type: 'string' },
    baseSha: { type: 'string' },
    commitSha: { type: 'string' },
    commitSubject: { type: 'string' },
    filesChanged: { type: 'array', items: { type: 'string' } },
    testsAdded: {
      type: 'array',
      items: { type: 'string' },
      description: 'Test function names added or changed',
    },
    failBeforeProof: {
      type: 'string',
      description:
        'How the new test was proven to fail without the fix — reverting the production change in place, not running against main',
    },
    contractChanges: {
      type: 'array',
      items: { type: 'string' },
      description:
        'Existing tests whose assertions were changed, and why — a reviewer must scrutinise each one',
    },
    weakPoints: {
      type: 'array',
      items: { type: 'string' },
      description: 'Seams the developer is least confident about, for the reviewer to attack',
    },
    siblingPrs: {
      type: 'array',
      items: { type: 'string' },
      description: 'Open pull requests touching the same lines, and whether the overlap is textual or a design conflict',
    },
    skippedChecks: {
      type: 'array',
      items: { type: 'string' },
      description: 'Checks not run, each with the reason — unavailable live infrastructure counts',
    },
    evidence: {
      type: 'array',
      items: { type: 'string' },
      description: 'Commands run with their exit codes',
    },
  },
  required: ['committed', 'reason'],
}

const REVIEW = {
  type: 'object',
  properties: {
    published: { type: 'boolean' },
    prNumber: { type: 'number' },
    prUrl: { type: 'string' },
    verdict: {
      type: 'string',
      description: 'ready-for-human-review | changes-made | blocked | withheld',
    },
    blockers: { type: 'array', items: { type: 'string' } },
    weakPointVerdicts: {
      type: 'array',
      items: { type: 'string' },
      description: "One entry per weak point the developer flagged, with the reviewer's finding",
    },
    botCoverage: {
      type: 'array',
      items: { type: 'string' },
      description: 'Per bot: whether it reviewed THIS head, verified by the commit_id on the review record',
    },
    checkState: { type: 'string' },
    skippedChecks: { type: 'array', items: { type: 'string' } },
    summary: { type: 'string' },
  },
  required: ['published', 'verdict', 'summary'],
}

function devPrompt(issue, sessionUrl) {
  const base = issue.base
    ? `Current \`origin/main\` is **\`${issue.base}\`**, fetched at dispatch — do not re-fetch unless you need to.`
    : `Fetch \`origin/main\` once and branch from it; record the SHA you used as \`baseSha\`.`
  const prior = issue.priorBranch
    ? `\n- **Prior unpushed work exists:** ${issue.priorBranch}. Read that diff first in one call. If it still applies to the base and is sound, reuse it rather than reimplementing; otherwise treat it as a reference. Do not modify that old worktree.`
    : ''
  const notes = issue.notes ? `\n\n## Front-loaded facts (do not rediscover these)\n\n${issue.notes}` : ''
  const scope = issue.scope ? `\n\n## Scope limit\n\n${issue.scope}` : ''

  return `Implement ${issue.repo} issue #${issue.num} test-first, and stop at a signed local commit. Do not push and do not open a pull request.

## Issue

${issue.repo}#${issue.num} — ${issue.title || '(title not supplied; read it from GitHub)'}
${issue.url || `https://github.com/${issue.repo}/issues/${issue.num}`}

${issue.body ? `### Issue text (verbatim, no need to fetch it)\n\n${issue.body}\n` : 'Read the issue and its comments in one batched `gh` call.'}
${notes}${scope}

## Working rules

- Repository git dir: \`${issue.gitDir || `repos/${(issue.repo || '').split('/').pop()}`}\`. ${base}
- Create your own worktree under \`${issue.worktreeRoot || '/tmp'}\` and work only inside it.${prior}
- **First decide whether the defect is still live on the base.** If a merged
  change already fixed it, stop and return \`committed: false\` with the merged
  pull request or commit as the reason. Do not manufacture a change to have
  something to hand over.
- Test-first: write the failing test that names the defect, prove it red, then
  make it pass with the smallest change. **Prove fail-before by reverting the
  production change in place** — not by running the test against \`main\`, where
  it may not even compile.
- If you change an existing test's assertion, record it in \`contractChanges\`
  with the reason. That is a deliberate contract change and a reviewer must see it.
- Read the consumers of any shape you change before changing it.
- \`golangci-lint\`'s cache is shared across worktrees here: a finding pointing
  into another checkout, or at a line that already has \`//nolint\`, is stale.
  Re-run with an isolated \`GOLANGCI_LINT_CACHE\`.
- Attribute any lint or test failure by running the same command at both your
  head and the base before calling it yours.
- Live Cardano infrastructure, devnet, conformance replays and Antithesis are
  **not available**. Record each as a skipped check with that reason. Never claim
  an unrun check ran.

${attribution(sessionUrl)}

${EFFICIENCY}

## Return value

Return the structured handoff. \`committed: false\` with a clear \`reason\` is a
correct and expected outcome when no change is needed — it is not a failure.
When \`committed\` is true, \`worktree\`, \`branch\`, \`baseSha\`, \`commitSha\` and
\`failBeforeProof\` must all be filled in, because the review stage is built from
them.`
}

function shepherdPrompt(issue, h, publish, sessionUrl) {
  const list = (label, items) =>
    items && items.length ? `\n### ${label}\n\n${items.map((v) => `- ${v}`).join('\n')}\n` : ''

  const publication = publish
    ? `## Publication

Push the branch and open the pull request against \`${issue.repo}\` \`main\`.
Keep the body short and factual: the defect, the change, the tests and their
fail-before proof, any sibling-PR merge note, and the skipped checks. No
storytelling, no roadmap, no chat transcript. End the description with:

\`\`\`
🤖 Generated with [Claude Code](https://claude.com/claude-code)
${sessionUrl ? `\n${sessionUrl}\n` : ''}\`\`\`

Do not merge it. Only the author merges their own pull request, and it still
needs human review.`
    : `## Publication is NOT authorized on this run

Review the change fully, but **stop before \`git push\` and before
\`gh pr create\`**. Publishing is outward-facing and this run did not authorize
it. Return \`published: false\` with \`verdict: "withheld"\`, plus everything the
operator needs to publish it later: the blockers you found, the fixes you made
locally, and the PR body you would have used in \`summary\`.

Fixing what you find locally, with signed commits on the same branch, is in
scope. Pushing is not.`

  return `Take a completed blink-tdd-developer branch for ${issue.repo} issue #${issue.num} through pre-publication review${publish ? ', publish it as a pull request, and own it until no actionable bot findings remain and it is ready for human review' : ''}.

## The handoff

- Issue: ${issue.repo}#${issue.num} — ${issue.title || ''}
  ${issue.url || `https://github.com/${issue.repo}/issues/${issue.num}`}
- Worktree: \`${h.worktree}\`
- Branch: \`${h.branch}\`
- Base: \`${h.baseSha}\`
- Commit: \`${h.commitSha}\` ${h.commitSubject ? `\`${h.commitSubject}\`` : ''}
- What it does: ${h.reason}
${list('Files changed', h.filesChanged)}${list('Tests added', h.testsAdded)}
${h.failBeforeProof ? `### Fail-before proof the developer reported\n\n${h.failBeforeProof}\n` : ''}${list('Existing test assertions the developer changed — scrutinise every one', h.contractChanges)}${list('Weak points the developer flagged — attack these, do not accept them', h.weakPoints)}${list('Sibling pull requests touching the same lines', h.siblingPrs)}${list('Checks the developer skipped, and why', h.skippedChecks)}
## Review it before you trust it

The handoff above is the developer's own account of its work. Verify the parts
that matter rather than restating them:

- Re-run the fail-before yourself for at least one new test, by reverting the
  production change in place.
- Read the code that any changed contract touches, not just the diff.
- Audit the **class** the change belongs to, not only the lines it edited —
  including any member the fix makes reachable for the first time.
- Verify Conventional Commits with no issue number in the subject, a subject of
  72 characters or fewer, DCO sign-off, and a GPG signature.

## Environment facts (front-loaded — do not rediscover)

- \`golangci-lint\`'s cache is shared across worktrees here: a finding pointing
  into another checkout, or at a line that already carries \`//nolint\`, is stale.
  Re-run with an isolated \`GOLANGCI_LINT_CACHE\`.
- Attribute any red check by running the linter at both \`${h.commitSha}\` and
  \`${h.baseSha}\` rather than reading CI logs.
- **Branch protection dismisses approvals on every push, merge commits
  included.** Batch a whole review round into one push, then re-request review
  explicitly through GitHub.
- Bot coverage is per-head and erratic. Verify CodeRabbit and Cubic actually
  reviewed **this head** by the \`commit_id\` stamped on each review record — not
  by check colour, and not by \`output.summary\` alone. An empty CodeRabbit review
  body is not a review. Document a quota-blocked or rate-limited bot explicitly;
  bot silence is never a clean review, and bot approval is never human approval.
- Live Cardano infrastructure, devnet, conformance replays and Antithesis are
  **not available**. Carry the developer's skipped checks into your report and
  add your own. Never claim an unrun check ran.

${publication}

${attribution(sessionUrl)}

${EFFICIENCY}

## Return value

Return the structured review. \`botCoverage\` must say, per bot, whether it
reviewed this head and how you established that.`
}

const issues = (args && args.issues) || []
if (!Array.isArray(issues) || issues.length === 0) {
  log('No issues supplied. Pass args.issues as [{repo, num, title, base, worktreeRoot, ...}].')
  return { issues: 0, results: [] }
}

const publish = !!(args && args.publish)
const sessionUrl = (args && args.sessionUrl) || ''

log(
  `${issues.length} issue(s). Publication ${publish ? 'AUTHORIZED — shepherds will push and open PRs' : 'withheld — shepherds stop before push'}.`,
)

const results = await pipeline(
  issues,
  (issue) =>
    agent(devPrompt(issue, sessionUrl), {
      agentType: DEV,
      label: `tdd:${issue.repo}#${issue.num}`,
      phase: 'Implement',
      schema: HANDOFF,
    }),
  (handoff, issue) => {
    if (!handoff) {
      log(`${issue.repo}#${issue.num}: developer agent returned nothing — no review dispatched.`)
      return { issue, outcome: 'developer-failed' }
    }
    if (!handoff.committed) {
      log(`${issue.repo}#${issue.num}: no commit — ${handoff.reason}`)
      return { issue, handoff, outcome: 'no-change' }
    }
    if (!handoff.branch || !handoff.commitSha || !handoff.worktree) {
      log(
        `${issue.repo}#${issue.num}: developer reported a commit but omitted branch/commitSha/worktree — cannot build a review handoff.`,
      )
      return { issue, handoff, outcome: 'handoff-incomplete' }
    }
    return agent(shepherdPrompt(issue, handoff, publish, sessionUrl), {
      agentType: SHEP,
      label: `pr:${handoff.branch}`,
      phase: 'Shepherd',
      schema: REVIEW,
    }).then((review) => {
      if (!review) {
        log(`${issue.repo}#${issue.num}: review agent returned nothing; branch ${handoff.branch} is unreviewed.`)
        return { issue, handoff, outcome: 'review-failed' }
      }
      log(
        `${issue.repo}#${issue.num}: ${review.published ? `published ${review.prUrl || `#${review.prNumber}`}` : 'reviewed, not published'} — ${review.verdict}`,
      )
      return { issue, handoff, review, outcome: review.published ? 'published' : 'reviewed' }
    })
  },
)

const settled = results.filter(Boolean)
const dropped = results.length - settled.length
const by = (name) => settled.filter((r) => r.outcome === name)

// No silent caps: every issue is accounted for in exactly one bucket.
log(
  `Done. published=${by('published').length} reviewed=${by('reviewed').length} ` +
    `no-change=${by('no-change').length} developer-failed=${by('developer-failed').length} ` +
    `review-failed=${by('review-failed').length} handoff-incomplete=${by('handoff-incomplete').length}` +
    (dropped ? ` pipeline-dropped=${dropped}` : ''),
)

return {
  issues: issues.length,
  publishAuthorized: publish,
  published: by('published').map((r) => ({
    issue: `${r.issue.repo}#${r.issue.num}`,
    pr: r.review.prUrl || r.review.prNumber,
    verdict: r.review.verdict,
    blockers: r.review.blockers || [],
    botCoverage: r.review.botCoverage || [],
    checkState: r.review.checkState,
  })),
  reviewedNotPublished: by('reviewed').map((r) => ({
    issue: `${r.issue.repo}#${r.issue.num}`,
    branch: r.handoff.branch,
    worktree: r.handoff.worktree,
    commitSha: r.handoff.commitSha,
    verdict: r.review.verdict,
    blockers: r.review.blockers || [],
    summary: r.review.summary,
  })),
  noChange: by('no-change').map((r) => ({
    issue: `${r.issue.repo}#${r.issue.num}`,
    reason: r.handoff.reason,
  })),
  needsAttention: settled
    .filter((r) => ['developer-failed', 'review-failed', 'handoff-incomplete'].includes(r.outcome))
    .map((r) => ({
      issue: `${r.issue.repo}#${r.issue.num}`,
      outcome: r.outcome,
      branch: (r.handoff && r.handoff.branch) || null,
    })),
  skippedChecks: settled.flatMap((r) => [
    ...(((r.handoff && r.handoff.skippedChecks) || []).map((s) => `${r.issue.repo}#${r.issue.num} (dev): ${s}`)),
    ...(((r.review && r.review.skippedChecks) || []).map((s) => `${r.issue.repo}#${r.issue.num} (review): ${s}`)),
  ]),
}
