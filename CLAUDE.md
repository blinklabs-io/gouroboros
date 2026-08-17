# Claude Code instructions

This workspace is shared by Claude, Codex, CodeRabbit, and Cubic. Use the same
repository boundaries, review standards, and durable documentation regardless
of which agent or review bot started the work.

## Start here

Read [`AGENTS.md`](AGENTS.md) first. For Go repositories and cross-repository
reviews, read the [Go repository common-ground guide](docs/go-repository-guide.md).
Then read the target submodule's local `AGENTS.md`, `CLAUDE.md`,
`CONTRIBUTING.md`, README, and Makefile.

The parent repository is a workspace of independently versioned submodules.
Source changes belong in the relevant submodule; the parent normally records
the resulting pointer and workspace documentation. Do not modify nested
repositories or re-pin submodules incidentally.

## Planning and tracking

Plan files and planning notes are local, ephemeral working artifacts. Never
commit them. Use repository issues for durable scope, acceptance criteria, and
follow-up tracking.

## Reviews and review bots

CodeRabbit and Cubic findings are review inputs, not substitutes for inspecting
the current checkout. For each finding:

1. Locate the exact path and current line or symbol.
2. Check the target repository's local instructions and existing tests.
3. Reproduce or disprove the behavior with a focused test, static check, or
   direct code path analysis.
4. Classify it as a merge blocker, non-blocking recommendation, false positive,
   or already addressed finding.

Do not copy bot prose into durable documentation without verifying it. When
reviewing a change, report behavioral and security defects first, then API or
wire compatibility, missing contract-specific tests, architecture boundaries,
documentation/generated drift, and style issues.

The review sequence is bot review first, human review second. Run CodeRabbit
and Cubic when configured, address their actionable findings, and only then
request human review. A human review is mandatory and may be AI-assisted, but
bot approval or silence never counts as human approval.

When a human reviewer requests changes, implement and validate the fixes,
summarize the changes on the pull request, and explicitly request another
review from that same person through GitHub. Do not assume that replying to
the review or pushing commits automatically completes the review loop.

## Go and Cardano work

Use repository-native Makefile targets and inspect every nested `go.mod`.
Preserve generated-code provenance, shared `ouroboros-mock` fixtures, raw CBOR
bytes, Cardano era semantics, and conformance vectors. For Dingo, also follow
the [Dingo maintainer skill](skills/dingo-maintainer/SKILL.md) and its local
architecture/database documentation.

Do not claim that a check ran when it required unavailable live GitHub,
registry, Cardano devnet, conformance, or Antithesis infrastructure. Record
the exact skipped check and reason in the handoff.

Use canonical upstream repositories and Go modules for source dependencies.
Blink Labs forks are emergency-only exceptions that require explicit approval,
an issue, and an exit plan; Apollo must use `Salvionied/apollo` under normal
circumstances.

## Commits

Use Conventional Commits and DCO sign-off (`git commit -s`). Keep workspace
documentation, skill, and submodule-pointer changes easy to distinguish.
