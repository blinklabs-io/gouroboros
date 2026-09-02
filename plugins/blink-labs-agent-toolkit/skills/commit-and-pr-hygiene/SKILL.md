---
name: commit-and-pr-hygiene
description: Write Blink Labs commits, pull request descriptions, issues, review comments, and release plans that meet organization policy — Conventional Commits, DCO sign-off, short factual messages, deliberate SemVer/tag boundaries, no committed plan files, milestones left off new issues because they carry an on-chain governance commitment, and submodule pointer updates kept separate from source changes. Use before every commit or release, when opening or updating a pull request, when opening or triaging an issue, or when writing review comments.
---

# Commit and PR Hygiene

Policy in this organization is narrow and enforced. Getting it wrong costs a
round trip, and in a submodule it can cost a bad pointer.

## Direct pushes are clanker-only; product repos need a PR

Push straight to `main` in the **clanker workspace superrepo only** — and only
for its own workspace docs, the `plugins/` toolkit, and submodule-pointer
updates. **Every other repository requires a pull request and the full review
loop for any source change.** The `repos/*` product submodules (Dingo,
gouroboros, txtop, cardano-up, bursa, plutigo, and the rest — all public) are
never committed or pushed to directly, their branch protection is never
bypassed, and this holds however small, urgent, or "obvious" the change is.
A release blocked on a code fix waits on that PR through bot-then-mandatory-
human review; it does not get a shortcut to a product repo's `main`. When a
release or re-release needs a source change (a Makefile, a workflow, a build
tag), open a PR for it — do not hand a subagent a direct-push instruction.

## Delegation does not lift a publication gate

When delegated work is held for parent review, keep every named gate in force:
no commit, push, issue or PR mutation, merge, tag, or release until the parent
explicitly releases that action. Review feedback, requested test improvements,
or a worker reporting completion authorizes only that requested work; it is not
implicit approval to publish. The parent must inspect the shared-worktree diff
and current remote state before issuing the publication instruction.

## Published branch history is immutable

A branch becomes published history at its first push. After that point, never
rebase it, amend its commits, reset it to a different history, or force-push it.
This applies even when no review has arrived yet and even when force-with-lease
would technically be safe.

- Being behind `main` is not a defect. Check mergeability, the merge tree,
  touched paths, and semantic overlap. If intervening commits do not affect the
  change, leave the branch alone.
- When a real conflict or direct compatibility dependency requires current-base
  integration, merge the base branch into the feature branch with a normal
  additive merge commit. Give it a Conventional Commit subject and DCO sign-off,
  then push normally.
- For stacked pull requests, merge the updated parent branch into the published
  child branch; do not rebuild the child with a rebase.
- Rebase only unpublished local work before its first push.

Never trade reviewer continuity and durable commit references for a tidier
published graph.

## Tag-only publication; never create releases manually

For every Blink Labs repository, the agent's publication action is **only** to
create the planned annotated/lightweight Git tag using the repository's
documented process. Never call `gh release create`, the Releases API, or an
equivalent release-creation command. Do not delete a release to correct a
mistake: GitHub can retain an immutable release record for the tag name and
prevent the workflow from publishing it. If a release object already exists or
a publish run fails, stop, record the exact workflow error, and ask the owner
for the recovery decision.

**Never manually release. Ever.** Release objects, generated release notes,
and package publication are owned by repository automation or the owner; an
agent must not create, edit, delete, or repair them.

Before tagging, verify the repository's publish workflow and confirm its
release-creation behavior. A tag is not evidence that the automated release or
package publication succeeded; after tagging, inspect the workflow and
consumer-visible artifact, but do not create or repair the release manually.

## Plan releases; do not tag every eligible merge

Merge eligibility and dependency readiness are not release boundaries. Before
creating a tag, inspect the latest released version, classify the accumulated
changes, list the intended release contents, check for other compatible changes
that should land in the same release, and identify the consumers that will move
to it. General authorization to create tags does not skip this planning step.

- Bundle compatible fixes into a planned patch release instead of publishing
  one version per merged pull request.
- In pre-1.0 Go modules, use a patch bump for backward-compatible bug fixes,
  tests, documentation, and internal refactors. Use a minor bump only for an
  intentional additive feature/API boundary or a compatibility change, and
  record that rationale in the private release handoff.
- Prefer one released-tag consumer update for the planned contents. Do not make
  downstream repositories chase a sequence of intermediate tags when they can
  consume the completed patch release.
- If a dependency fix is urgent enough to release alone, state that exception
  and its version rationale before tagging; urgency does not automatically turn
  a patch into a minor release.
- A published tag or release is durable external state. Never delete, move, or
  replace it without explicit user direction. Correct subsequent contents with
  the next SemVer version.

After publishing the planned tag, verify the tag target, release workflow,
package/proxy publication, and consumer-visible version before calling the
release usable.

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
  If the user does authorize skipping, record it only in the private task
  handoff when it is operationally useful. Do not put the signing mode,
  exception, unsigned-commit status, or re-signing instructions in a pull
  request description, issue, or review comment unless the user explicitly
  asks or a repository check exposes it as a concrete blocker. Those are
  session mechanics, not code-review facts. The same authorization rule applies
  to `git rebase`, which fails the same way mid-operation and leaves the rebase
  half-applied; `-c commit.gpgsign=false` is available but is subject to the
  same permission.
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
`git diff --cached --stat` before committing. Then read
`git diff --cached --name-status` and account for every staged path; a plausible
stat total does not reveal an accidentally staged tracking report or gitlink.

## Never commit

Plan files, planning notes, local audit or tracking reports, scratch analyses,
session handoffs, agent state, credentials, tokens, private configuration, build
artifacts, and generated files the project does not track. Plans and local
tracking files are ephemeral working artifacts even when they contain public
issue or pull-request links. When work needs durable tracking, open or update an
issue with scope, acceptance criteria, and context. Commit one of these files
only when the user explicitly identifies it as a deliverable.

## Write multiline GitHub bodies through files

For a multiline issue, pull-request, review, or comment body, write real
Markdown to a run-owned `.md` file and pass that file with `--body-file` or a
REST input file. Do not pass JSON-stringified Markdown or escaped newlines as a
shell argument: the literal `\n` characters can be stored in the public body
instead of becoming line breaks.

After every create or edit, read the complete live record back through GitHub.
Check the title, body, and requested metadata such as assignees, labels, and
milestone; verifying one field is not verification of the others. Confirm that
headings and lists have real line breaks and that the body contains no literal
newline escapes before claiming the write succeeded. Correct the same record
when verification fails, then read it back again. Remove the run-owned body
file after the live record is verified.

## Pull requests

- A PR description may contain only facts needed to review the current diff:
  the defect or requirement (or a link to its issue), what the code changes,
  the resulting behavior or guarantee, and a validation fact only when it
  materially changes the risk assessment. Delete every other sentence.
- Omit release and tag chronology, branch or base movement, work-session
  history, signing choices, bot status, rollout sequencing, and future
  follow-up notes. In particular, never explain that a PR is a follow-up to,
  predates, or will land after a release tag; the commit graph already records
  that history, and it does not help review the diff.
- Do not mechanically add `Summary`, `Testing`, or other template sections. If
  the title, diff, and linked issue provide all necessary context, use an empty
  body instead of filler.
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
- Re-read the live description through GitHub after creating or editing the PR,
  after requesting reviewers, and after each bot pass. Cubic and CodeRabbit can
  append generated HTML after the authored body was already verified, so a
  clean local body file or pre-bot read is not evidence that the live body is
  still clean. Strip new wrappers and read the result back again.
- UI changes require screenshots in the PR. Include the affected states and
  relevant viewport or platform; redact secrets and user data. Determine this
  from rendered behavior, not merely from a file living under `src/`: a data
  edit to fields no UI consumer renders does not require screenshots. Trace the
  consumers before claiming the rendered site is unchanged.
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

## Milestones are a governance commitment, not a backlog label

**Open new issues with no milestone.** Leave the field empty unless the issue is
required by a milestone's already-committed scope.

Milestones in this organization are linked to the Dingo treasury withdrawal
governance action on Cardano. Their scope is fixed and was committed on chain,
and it covers **block production**. A milestone is therefore a statement about
what was funded, not a bucket for related work. Adding an issue to one restates
that commitment, so the default is to leave it off and let a human decide.

Out of scope for the block-production commitment, however plausibly related the
work looks:

| Out of scope | Notes |
|---|---|
| API mode | Serving APIs is not block production. |
| Cloud storage backends | S3, GCS, and other remote blob or snapshot stores. |
| Anything a block producer does not need to forge and diffuse blocks | The test is necessity, not adjacency. |

Rules that follow from this:

- The same milestone titles appear in more than one repository and feed the same
  commitment. `gouroboros` milestones are not a separate, looser namespace.
- Do not infer scope from a sibling issue. An existing issue carrying a
  milestone is not evidence that a new one belongs there; it may predate the
  commitment or have been placed deliberately by a human.
- Do not remove a milestone a human set. Adding and removing are both scope
  decisions.
- When an issue looks like it genuinely belongs, still open it unmilestoned and
  say so in the handoff, naming the milestone and why. One sentence to a person
  is cheap; an unasked-for change to committed scope is not.

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
- A failed normal merge is never authorization to bypass repository policy.
  Do not use `--admin`, an API policy override, or an equivalent bypass unless
  the owner explicitly authorizes that bypass for this specific pull request.
  If normal merging is refused, inspect the current policy and PR state, then
  stop and return the refusal to the owner.
- Use one concise factual squash summary. Do not concatenate commit bodies,
  review threads, or chat history.
- Set the squash subject to the exact PR title. Never invent a generic subject
  such as `Merge approved changes`, `Merge PR`, or `Squash merge`; those messages
  discard the purpose of the change and are blocked by the merge guard.
- Write the squash body as real Markdown in a run-owned file and pass it with
  `--body-file`; never encode its line breaks as `\\n` in `--body`. Lock the
  merge to the reviewed head with `--match-head-commit`.
- REST-only status or review checks do not change the merge transport: use
  `gh pr merge --squash --body-file ... --match-head-commit ...` for the merge.
  If the owner explicitly requires the REST merge endpoint, put the complete
  request in a validated JSON input file and use `gh api --input`; never pass a
  multiline `commit_message` through `-f`, `-F`, JSON-stringified shell text,
  or escaped newlines.
- Preserve the DCO `Signed-off-by:` line in the squash commit. After merging,
  read the resulting commit message back and verify the sign-off is a standalone
  trailer, not literal escaped text. Do not use a merge path that drops or
  corrupts the sign-off.
- Before merging, run `python3 plugins/blink-labs-agent-toolkit/scripts/validate-squash-merge.py`
  with the PR title, proposed subject, and body file. After merging, read the
  remote commit message and stop if its subject differs from the PR title or
  its DCO trailer is malformed.

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
