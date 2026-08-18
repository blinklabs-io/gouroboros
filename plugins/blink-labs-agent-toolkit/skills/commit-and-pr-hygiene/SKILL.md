---
name: commit-and-pr-hygiene
description: Write Blink Labs commits, pull request descriptions, and review comments that meet organization policy — Conventional Commits, DCO sign-off, short factual messages, no committed plan files, and submodule pointer updates kept separate from source changes. Use before every commit, when opening or updating a pull request, or when writing review comments.
---

# Commit and PR Hygiene

Policy in this organization is narrow and enforced. Getting it wrong costs a
round trip, and in a submodule it can cost a bad pointer.

## Commits

- [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) for
  every commit: `type(scope): summary`, where type is one of `build`, `chore`,
  `ci`, `docs`, `feat`, `fix`, `perf`, `refactor`, `revert`, `style`, `test`.
- DCO sign-off on every commit: `git commit -s`.
- GPG signing is required in every repository except `skunkworks/`.
- Subjects stay short and factual — the change, not the story.
- A body is optional. When present, it holds only short factual lines tied
  directly to the changed code, tests, or review. Never a chat transcript,
  narrative, roadmap, future plan, or unrelated context.
- One logical change per commit. Keep a submodule pointer update in its own
  commit, separate from source changes made inside the nested repository.

Correct:

```
fix(ledger): reject headers with a stale VRF proof
docs: clarify submodule pointer workflow
build: bump gouroboros to v0.x.y
```

Wrong: `update stuff`, `WIP`, `fix: fixed the thing I broke earlier while
investigating the chain selection issue we discussed`.

## Never commit

Plan files, planning notes, scratch analyses, session handoffs, agent state,
credentials, tokens, private configuration, build artifacts, and generated files
the project does not track. Plans are local, ephemeral working artifacts. When
work needs durable tracking, open or update an issue with scope, acceptance
criteria, and context.

## Pull requests

- Description: short, factual, scoped to this change. What changed, why, what
  was validated, what was deliberately not validated.
- Put code-specific feedback in inline comments. Reserve PR-level comments for
  concise summaries, check results, and dispositions.
- List skipped checks explicitly. An unavailable devnet, registry, or CI
  dependency is a fine answer; a silent omission is not.

## Review loop

The sequence is bots first, humans second, and it does not compress:

1. Do not review draft PRs. If the user excludes Dependabot, omit PRs authored
   by `dependabot[bot]`.
2. Verify the current PR head, requested reviewer, checks, and latest bot
   findings before reviewing. Run or wait for configured CodeRabbit and Cubic
   reviews.
3. Reproduce or disprove each finding against the current checkout, then
   classify it: merge blocker, non-blocking recommendation, false positive, or
   already addressed. Do not copy bot prose into durable documentation.
4. Request the required human review. Bot approval or silence is never human
   approval, and human review is mandatory even when AI-assisted.
5. When a human requests changes: implement and validate the fixes, summarize
   them on the PR, and explicitly request another review from that same person
   through GitHub. Replying or pushing does not close the loop.
6. An authorized human reviewer may dismiss another human review through GitHub;
   record the rationale on the PR. Dismissal never removes the requirement for
   appropriate human review.

Do not claim a review, approval, re-request, or dismissal happened unless GitHub
shows it.

## Submodule work

1. Make and validate the change in the submodule.
2. Commit it there, signed off and conventionally named.
3. Update the parent pointer in `clanker` as a separate, clearly labeled commit.

Do not create a parent-repository commit on the user's behalf unless explicitly
asked, and never re-pin submodules the task did not name.
