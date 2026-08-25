# Module audit rules

Use:

```sh
rg --files repos -g 'go.mod' -g '!**/.git/**' | sort
rg -n --glob 'go.mod' --glob '!**/.git/**' \
  '^(module|go |toolchain|replace)|github.com/blinklabs-io/' repos
```

For each module record:

1. module path and Go/toolchain floor;
2. direct and indirect Blink dependencies;
3. active and commented local replacements;
4. nested-module boundary and owning repository;
5. matching workspace submodule or external canonical source;
6. module-path and repository-URL consistency.

For a pinned-build or security review, prove source identity before trusting an
inventory or directory name:

1. record `git rev-parse HEAD` and resolve the requested tag through
   `refs/tags/<tag>^{}` so annotated tags and commits compare correctly;
2. record the module path, `go`/`toolchain` directives, and the filesystem root
   embedded in any generated inventory or summary;
3. reject a manifest as exact-pin evidence if its source root or revision does
   not match, even when its filename contains `pinned` or the expected version;
4. keep a maintained-head comparison as a separately labeled delta rather
   than merging its declaration/test counts into the pin; and
5. run each nested module and example against its own `go.mod`. Use `GOWORK=off`
   when a parent workspace could replace the graph being audited.

When an exact toolchain is required, prefer the corresponding published
`ghcr.io/blinklabs-io/go` image, record its resolved digest and platform, and
set `GOTOOLCHAIN=local` inside it. Give the container writable, run-owned Go
caches; a read-only cache failure is an environment limitation, not a package
test result.

Normalize semantic-major suffixes when mapping paths (`/v2` remains part of
the module identity, not the checkout directory). Treat generated OpenAPI,
UI, example, and Antithesis modules independently.

## Dependency-update isolation

Run graph inspection and consumer validation with `GOWORK=off` from the module
being changed. Record `go list -m all` or `go mod graph` before and after the
update so a transitive fixture, mock, or conformance-module change is visible
instead of being attributed only to the direct dependency named in `go.mod`.

When the updated graph changes behavior, use isolated worktrees or temporary
modules to test the smallest useful matrix: old direct/old transitive, new
direct/new transitive, and mixed pairs when module constraints allow them. A
newer fixture may expose a defect that the older fixture accidentally hid. In
that case, fix and release the owning rule, then consume released tags through
the graph; do not restore green by pinning the stale fixture or by committing a
diagnostic `replace`.

Report missing or suspicious entries as a table with module, requiring file,
expected source, current version, and recommended action. Do not silently add a
fork, replacement, or submodule to make the graph appear complete.
