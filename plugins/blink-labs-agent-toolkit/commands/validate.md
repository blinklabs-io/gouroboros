---
description: "Run the narrowest meaningful checks for a changed Blink Labs or TosiDrop repository and report an evidence ledger of what ran, passed, failed, and was skipped"
argument-hint: "[repository path or 'all changed']"
allowed-tools: ["Bash", "Glob", "Grep", "Read", "Task"]
---

# Validate an owned-repository change

Scope: "$ARGUMENTS" (if empty, validate every repository with changes in
`git status --short --recurse-submodules`).

Follow the `evidence-based-handoff` and `isolated-validation-runs` skills. Never
report a check as run unless you have its command line, exit code, and output.

## Steps

1. **Determine the changed repositories and the changed contracts.** A change to
   a `.proto`, OpenAPI spec, SQL file, Dockerfile, or CBOR-adjacent type has a
   wider blast radius than its package.

2. **Prefer repository-native targets.** Read the `Makefile` first. Use its
   targets (`make test`, `make lint`, `make import-boundaries`, `make sql-check`,
   `make gorm-check`, `make docs-parity`) before inventing a command.

3. **Start narrow, then widen** as the affected contract requires:
   - Go: `gofmt -l`, focused `go test -run`, `go test ./...`, `go vet ./...`,
     `golangci-lint run`, `nilaway`, race tests for concurrent code. Run tests in
     every nested module, not only the root.
   - Docker: `docker build --check .` and `actionlint` on changed workflows. Do
     not push images or manifests.
   - Infrastructure: `ansible-lint`, `helm lint`, `helm template`,
     `terraform fmt -check`, `terraform validate` per module.
   - Docs site (`repos/docs-site`): `npm ci`, `npm run check`, `npm run build`.
   - TosiDrop: select the exact profile from
     `tosidrop-repo-maintainer/references/repository-map.md`. In particular,
     `web` uses `npm ci`, tests, lint, and build; `vm-sdk` also checks its
     package surface; `vm-frontend` has separate client/server lockfiles and
     some no-op test scripts; `infrastructure` requires Terraform validation
     and authorized state access for plans.
   - Workspace: `git diff --check`, `git diff --submodule=log`,
     `git submodule status --recursive`.

4. **Isolate long or live runs.** Use a worktree, unique ports, unique temporary
   paths, and a unique `GOCACHE`. Serialize sync/from-genesis runs. For a
   suspicious failure, reproduce it against an `origin/main` baseline before
   attributing it to the change.

5. **Inspect background tasks completely.** A completion notification is not a
   result. Read the full output and the exit code, and confirm the intended gate
   actually ran.

## Report

Emit a table with one row per check: command, working directory, exit code,
verdict (pass / fail / pre-existing failure / skipped), and evidence pointer.
List every skipped check with the concrete reason — unavailable devnet, no
registry credentials, no live GitHub, no Antithesis access. Never describe live
devnet, conformance, registry, or Antithesis validation as having run when it
did not.
