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
- GPG signing is required in every repository except `skunkworks/`. When signing
  fails because pinentry cannot prompt — `gpg: signing failed: No passphrase
  given` — stop and report it. Suggest the user unlock the key in the session
  (`! echo test | gpg --clearsign > /dev/null`). Never reach for
  `--no-gpg-sign` on your own: dropping a required signature is the user's call,
  and an unsigned commit on a branch of signed ones has to be rewritten to fix.
  If the user does authorize skipping, say so in the handoff and note that the
  commits will need re-signing before merge. The same applies to `git rebase`,
  which fails the same way mid-operation and leaves the rebase half-applied;
  `-c commit.gpgsign=false` is available but is subject to the same permission.
- Subjects stay short and factual — the change, not the story.
- A body is optional. When present, it holds only short factual lines tied
  directly to the changed code, tests, or review. Never a chat transcript,
  narrative, roadmap, future plan, or unrelated context.
- One logical change per commit. Keep a submodule pointer update in its own
  commit, separate from source changes made inside the nested repository.
- Let the repository's own `git config user.name` / `user.email` set the author.
  Do not pass `-c user.email=` or `--author=` on a hunch about which address is
  "the user's": the DCO trailer has to match the other commits on the branch,
  and a mismatch is only fixable by rewriting and force-pushing. Check with
  `git log -3 --format='%an <%ae>'` if unsure. A workspace guard warns on an
  override that disagrees with the configured identity.
- Merge commits need a Conventional Commit subject too. Git's default
  `Merge remote-tracking branch 'origin/main' into <branch>` is rejected by the
  workspace guard; use `chore: merge origin/main into <branch>` and put the
  conflict resolutions in the body. GitHub's Conventional Commits check may
  exempt merges where the guard does not — write for the guard.

Correct:

```
fix(ledger): reject headers with a stale VRF proof
docs: clarify submodule pointer workflow
build: bump gouroboros to v0.x.y
```

Wrong: `update stuff`, `WIP`, `fix: fixed the thing I broke earlier while
investigating the chain selection issue we discussed`.

## Stage explicitly

Name the paths you are committing. Never `git add -A`, `git add .`, or
`git commit -a`: a workspace checkout routinely carries changes that are not
yours to commit — a submodule pointer moved by a checkout, another session's
edits, local agent state — and a blanket add sweeps them in silently. Run
`git status --short` first, add the files the change actually touches, and check
`git diff --cached --stat` before committing.

## Never commit

Plan files, planning notes, scratch analyses, session handoffs, agent state,
credentials, tokens, private configuration, build artifacts, and generated files
the project does not track. Plans are local, ephemeral working artifacts. When
work needs durable tracking, open or update an issue with scope, acceptance
criteria, and context.

## Pull requests

- The description is a code-review brief, not an agent handoff. State the
  defect or requirement, what the code changes, and the resulting behavior or
  guarantee. Prefer a few concrete bullets over process prose.
- Do not paste a command inventory, evidence ledger, or skipped-check list into
  the description. Those belong in the task handoff. Mention a validation
  result only when it materially changes how a reviewer should judge the code
  or its risk.
- Keep a bot summary only when it adds concise code facts that are missing from
  the authored description. Normalize those facts into plain Markdown and
  remove attribution wrappers, raw HTML, buttons, hidden bot state, prompts,
  run IDs, and stale commit metadata.
- UI changes require screenshots in the PR. Include the affected states and
  relevant viewport or platform; redact secrets and user data.
- Put code-specific feedback in inline comments. Reserve PR-level comments for
  concise code summaries and review dispositions.
- Do not list out-of-scope work or checks that were not run. If a missing
  required check creates a concrete merge risk, state that risk in one factual
  sentence instead of adding a generic `Skipped` section.

### Never leak the conversation into a PR or issue

Anything posted to GitHub is a technical record for whoever reads it months from
now. It is not a reply to whoever asked for the work, and the chat that produced
it must not show through. Write about the code, not about the exchange.

Cut all of these before posting:

| Do not write | Why |
|---|---|
| "The decision is yours", "let me know", "want me to take it?" | Addresses a person who is not the audience. On our own PR the second person refers to nobody — the author is the account posting. |
| "Fixing this now", "next I'll…", "I'll follow up" | Status of a working session, not a property of the change. Post the fix, not the intention. |
| "I ran…", "I only checked…", "I was wrong about…" | Narrating the agent's process. State what holds about the code; attribute evidence to a command and its result. |
| "Correcting my own review", "as I said above" | Conversation bookkeeping. Edit the wrong text or post the corrected fact plainly. |
| "Good catch", "you're right", "great question" | Pleasantries carry no information. |

A finding reads as: the observation, the evidence, the disposition. Evidence is a
command and its output, or a `file:line` and what the code does there — not a
first-person account of looking for it. When something needs deciding, state the
options and the constraint that discriminates them, and then decide; do not hand
the decision to an unnamed reader.

Check the PR's author before writing a single word of second person. On a PR the
posting account authored, there is no "you" — describe the change, not a request
to its author.

This is the same standard as the no-storytelling rule for commit bodies. Prefer
editing a comment that broke it over posting another one that explains the first.

## Review loop

The sequence is bots first, humans second, and it does not compress:

1. Do not review draft PRs. If the user excludes Dependabot, omit PRs authored
   by `dependabot[bot]`.
2. Verify the current PR head, requested reviewer, checks, and latest bot
   findings before reviewing. Run or wait for configured CodeRabbit and Cubic
   reviews. If CodeRabbit is rate-limited, document it; a completed Cubic
   review is sufficient for bot review.
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

When asked to resolve an issue, continue through the initial implementation and
PR update, configured bot reviews, valid bot fixes, validation, and bot re-runs
until no actionable bot findings remain. The stopping point is readiness for
human review, not human approval or merge.

## Squash merge

- **Merge only your own pull requests.** Whoever presses merge takes
  responsibility for the code, so an approval is not ownership: hand an approved
  PR back to its author instead of merging it for them. The sole exception is
  `dependabot[bot]`, which cannot merge its own.
- Confirm GitHub shows a human `APPROVED` review whose commit SHA matches the
  current PR head. Approval of an earlier ref is stale after a new commit.
- Confirm required checks pass and configured bots have no actionable findings.
- Use one concise factual squash summary. Do not concatenate commit bodies,
  review threads, or chat history.
- Preserve the DCO `Signed-off-by:` line in the squash commit. Do not use a
  merge path that drops the sign-off.

## Submodule work

1. Make and validate the change in the submodule.
2. Commit it there, signed off and conventionally named.
3. Update the parent pointer in `clanker` as a separate, clearly labeled commit.

Do not create a parent-repository commit on the user's behalf unless explicitly
asked, and never re-pin submodules the task did not name.

### Read the submodule status codes before reacting

`git status --short` reports submodules with a leading space and a second-column
code, and they do not mean what the same letters mean for a file:

| Code | Meaning |
|---|---|
| ` M` | Tracked content differs — usually a moved pointer |
| ` ?` | The submodule contains **untracked files**; the gitlink is fine |
| ` m` | Modified content inside, not the pointer |

` ?` is not "the submodule is broken" or "the gitlink is gone", and it is not
something to repair. It routinely appears because someone has local worktrees or
build output inside the submodule — `repos/cdnsd/.worktrees/` is a real example,
and cdnsd has an open PR to gitignore exactly that. Settle it with
`git ls-files --stage <path>`: a `160000` line means the gitlink is present and
correct, whatever the status column says. `git -C <path> status --short` then
shows what the untracked content actually is.

Leave that content alone. Local worktrees belong to whoever made them, and a
stray directory inside a submodule is not yours to delete just because it makes
the parent's status noisy.
