---
name: blink-workspace-navigator
description: Find the repository that owns a topic, symbol, image, fixture, or workflow in the Blink Labs clanker workspace, and understand how the projects depend on each other. Use at the start of any cross-repository task, when the owning project is not obvious from a name, when a dependency is required but not checked out, or when deciding which repository a change belongs in.
---

# Blink Workspace Navigator

Use this skill to answer "where does this live and where does the change
belong?" before doing anything else. The workspace has more than fifty
repositories with deliberately similar names; a wrong guess costs a whole
review cycle.

Read [ownership-map.md](references/ownership-map.md) for the topic-to-repository
map and the dependency spine. Read the
[repository families reference](../blink-repo-maintainer/references/repository-families.md)
for the per-family validation profile.

## Ground rules

1. `clanker` is a workspace, not an umbrella project. Each `repos/<name>` entry
   has its own history, `Makefile`, CI, release process, and `CODEOWNERS`.
2. A name is a hint, not a fact. `docker-cardano-node` builds an image;
   `cardano-node-api` is a Go service; `dingo` is the node itself. Open the
   project's files before asserting what it does.
3. A missing feature and a missing checkout look identical to `grep`. Run
   `git submodule status --recursive` first; a leading `-` means the submodule
   was never initialized, and a leading `+` means its pointer has already moved.
4. Layered projects have layered ownership. Behavior seen in an application is
   often implemented in `gouroboros`, `plutigo`, or `ouroboros-mock`. Trace to
   the declaring module before proposing a fix in the consumer.

## Method

1. **Locate.** Search the workspace, not one repository. Map a Go symbol to its
   module by walking up to the nearest `go.mod`, then map the module path back
   to a `repos/` directory, normalizing any `/vN` suffix.
2. **Confirm ownership.** Check whether the file is generated. Generated
   workflow wrappers are outputs of `repos/actions`; generated Go clients come
   from an OpenAPI or protobuf source. The change belongs in the source.
3. **Find the consumers.** Search every checkout for importers before changing a
   shared library. Also list modules that require it but are not checked out —
   those are still consumers, just invisible locally.
4. **Place the change.** State the owning repository, whether the parent
   workspace needs a pointer update, and which local instructions govern the
   work. If a fixture is involved, shared protocol fixtures belong in
   `ouroboros-mock`.
5. **Report absence honestly.** If something is not in the workspace, say so
   rather than offering the closest-named repository as the answer.

## Historical session context

Local Codex and Claude session records can be searched by repository path for
prior investigations, validation commands, and recurring failure modes. Treat
them as history only: verify every conclusion against the current checkout, and
never copy credentials, untracked files, or stale fixes out of them. Distill
repeated, repository-independent procedures into a skill; leave one-off bug
detail in the owning repository's issues.
