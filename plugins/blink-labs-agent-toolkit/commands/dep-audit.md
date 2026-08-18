---
description: "Audit Go module graphs, nested modules, local replacements, and upstream-versus-fork provenance across the Blink Labs workspace"
argument-hint: "[repository path or 'workspace']"
allowed-tools: ["Bash", "Glob", "Grep", "Read", "Task"]
---

# Blink Labs dependency audit

Scope: "$ARGUMENTS" (if empty, audit every repository under `repos/`).

Read-only by default. Follow the `go-dependency-auditor` skill and its
`references/module-audit.md`. Do not run `go mod tidy`, rewrite module paths, or
add submodules as a side effect of an audit.

## Steps

1. Enumerate every `go.mod` recursively and record its module path and `go`
   directive. Treat each as an independent boundary.
2. Extract direct and indirect `github.com/blinklabs-io/` requirements, all
   `replace` directives (active and commented out), and each module declaration.
   Normalize `/vN` suffixes when mapping a module to a repository path.
3. Map every required repository to a submodule checkout under `repos/` or to an
   explicitly external dependency. Report a missing checkout separately from a
   missing Go module — they have different fixes.
4. Flag module-path mismatches, replacement directions that point at a local
   path, and versions that lag the workspace pointer.
5. Apply provenance policy: canonical upstream repositories and modules are the
   default. A Blink Labs fork is an emergency-only exception that needs explicit
   approval, an issue, and an exit plan. Apollo must be `Salvionied/apollo`
   under normal circumstances.

## Report

Give the dependency graph (who requires whom, at which version), then a table of
findings: missing checkouts, missing modules, active local replacements, path
mismatches, fork usage without an approved exception, and stale pins. Finish
with the ordered, minimal set of follow-up changes and which repository owns
each one. Propose changes; do not apply them unless the user asked.
