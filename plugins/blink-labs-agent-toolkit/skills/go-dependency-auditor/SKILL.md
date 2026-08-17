---
name: go-dependency-auditor
description: Audit Blink Labs Go module graphs, nested modules, local replacements, missing workspace checkouts, module-path mismatches, and upstream-versus-fork provenance. Use when adding repositories, reviewing dependency updates, changing go.mod/go.sum, or checking cross-repository buildability.
---

# Go Dependency Auditor

Use this skill for read-only dependency discovery and focused dependency
changes. Read the [module audit reference](references/module-audit.md) for the
normalization and reporting rules.

## Workflow

1. Enumerate every `go.mod` recursively; treat each as an independent module
   boundary and record its `go` directive.
2. Extract direct and indirect `github.com/blinklabs-io/` requirements,
   `replace` directives, and module declarations. Normalize `/vN` paths when
   mapping a module to a repository path.
3. Map each required repository to an existing submodule or an explicitly
   external dependency. Distinguish a missing checkout from a missing Go
   module and report both paths.
4. Check module identity, source URL, version, replacement direction, and
   whether a local replacement is active or commented out.
5. Apply provenance policy: use canonical upstream repositories and modules.
   Blink Labs forks are emergency-only, requiring explicit approval, an issue,
   and an exit plan. Apollo is external upstream-only under normal conditions.
6. Report a dependency graph and actionable follow-ups before editing. Do not
   run `go mod tidy`, rewrite module paths, or add submodules as a side effect
   of an audit unless the task explicitly includes that change.

When implementing an authorized dependency change, update the owning
repository's `go.mod`/`go.sum`, test each affected module, and keep the parent
workspace pointer separate from source changes.
