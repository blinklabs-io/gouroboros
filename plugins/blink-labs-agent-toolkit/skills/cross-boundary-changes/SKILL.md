---
name: cross-boundary-changes
description: Change a shape that crosses a boundary — an HTTP response and its SPA consumer, a Go interface and its callers, a datum decoder and its parser, a stored key format and its persisted data — without breaking the other side. Use before editing any response body, error path, struct field, ID format, or return contract, and whenever a review finding names only one side of such a boundary.
---

# Cross-Boundary Changes

Use this skill when a change is visible to something you are not editing in the
same breath. The recurring failure it exists to prevent is fixing one side of a
contract, shipping it, and discovering the other side never agreed.

## Find the consumer before you change the shape

Before editing a response body, an error path, a public field, an ID format, or
a return contract:

1. **Name every consumer.** Grep the whole workspace, not just the package. For
   an HTTP handler that means the API client *and* the screen that renders it;
   for a Go interface it means every implementation and every caller; for an
   on-disk format it means already-persisted data.
2. **Read the consumer's code.** Not its name, not its type — the lines that
   handle the field you are changing. A shared client helper that parses only
   `{"error": …}` makes any other field you add unreachable, whatever the
   server sends.
3. **Ask what the consumer does when the field is absent, empty, or false.**
   That is the case your change creates.
4. **Check for a second source of truth.** Two sides can independently assert
   the same fact, and then one of them is wrong: a handler that hardcodes
   `active: true` and a client that also hardcodes it will agree until the day
   the operation fails, and no test of either side alone will catch it.

If the consumer cannot act on what you are about to send, the change is not
finished — it is just moved.

## Decide the contract once

Work out what the response *means* before choosing its shape, and hold that
position:

- A request that created a resource is not a failure, whatever else went wrong
  afterwards. Returning 5xx for it invites a retry that hits a duplicate check.
- A partial success needs a status that says "done" and a body that says what
  is not done, not a status that says "failed" and a body that contradicts it.
- Additive JSON fields are safe; changed meanings of existing fields are not.

When a review comment pushes back, re-derive the answer from the contract
rather than adjusting to the comment. Changing the shape once per round of
feedback is the signature of reacting instead of deciding, and it costs a review
cycle every time.

## Read paths degrade, write paths fail

Two different rules, and using the wrong one is a real defect:

- **Read paths** — browsers, listings, dashboards, indexers over data another
  component owns — degrade per record. One value this build does not understand
  must not blank the whole view. Log it, skip it, keep the rest.
- **Write paths and migrations** — anything that deletes a source after copying,
  or advances a durable watermark — fail the batch. Continuing past a record you
  could not process risks destroying the only other copy or committing a partial
  state.

Ask which kind of path you are on before choosing. A node-local browser that
turns one unrecognized enum value into a 503 has picked the migration rule for a
read path.

## Boundaries in this workspace worth naming

- Go HTTP handler → API client helper → screen. Three hops; a shape change must
  survive all three.
- Decoder → parser → oracle/indexer handler → pipeline error channel. Check
  whether a new error is swallowed with `continue` or reaches a fatal path.
- Generated surfaces: OpenAPI, protobuf/Buf, sqlc. Regenerate rather than
  hand-edit, and review the whole generated diff. See
  [go-api-maintainer](../go-api-maintainer/SKILL.md).
- Protocol libraries → every consumer repository. A change in `gouroboros`,
  `plutigo`, or `ouroboros-mock` reaches repositories that are not checked out
  locally; see [go-dependency-auditor](../go-dependency-auditor/SKILL.md).
- ID and key formats → persisted state. Changing how a key is derived orphans
  everything already stored under the old one; check whether anything persists
  it before changing it.

## Merging a diverged branch is a boundary too

When `main` and a long-lived branch both touched the same contract, the merge is
where the two sides of that boundary meet — and git resolves text, not meaning.
Two failure modes, neither of which shows up as a conflict marker:

**A clean auto-merge that does not compile.** One side changed a signature and
the other added a call site. Both hunks apply, and the break appears only at
build time — possibly only under CI's build tags. So after every merge or
rebase, build and test with the repository's tag set before pushing. A clean
`git status` says nothing about whether the result compiles.

**A real conflict where both sides are right.** If each side hardened the same
code differently, "take ours" silently drops the other's fix. This is the
dangerous one, because the result compiles and the tests pass:

> A branch extracted VRF and opcert loading into a helper. Meanwhile `main`
> hardened the same read from `bursa.LoadKeyFromFile` to `loadSecretKeyFromFile`,
> adding regular-file, permission, and size checks for the secret key. Taking the
> branch's side kept the refactor and lost the hardening; taking `main`'s side
> kept the hardening and lost the refactor. The resolution had to be both: the
> extracted helper calling the hardened loader.

So for each conflict, read what *each* side was trying to accomplish before
choosing, and treat "one side is a security or correctness fix" as the signal to
combine rather than choose. Then state in the merge commit body which side each
resolution came from and why — a reviewer cannot see a resolution in the diff.

Prefer a merge to a rebase when the branch already contains merges from `main`,
when it will be squash-merged anyway, or when a rebase would replay many
commits: one resolution to reason about beats the same conflict re-appearing per
commit, and no force-push is needed.

## Report what you traced

State which consumers you read and what each does with the changed field. "The
SPA handles it" is not a finding; "`client.ts:128` reads only `j.error`, so the
added field is unreachable" is.
