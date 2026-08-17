# Blink Labs repositories

This directory contains Blink Labs repositories tracked as Git submodules.
Each submodule keeps its own history, issues, releases, license, contribution
guidance, and development tooling. This repository records the specific commit
that belongs in a workspace checkout.

## Current repositories

| Path | Repository | Purpose |
| --- | --- | --- |
| [`.github`](.github) | [`blinklabs-io/.github`](https://github.com/blinklabs-io/.github) | Organization-wide contribution, security, and community defaults |
| [`actions`](actions) | [`blinklabs-io/actions`](https://github.com/blinklabs-io/actions) | Reusable GitHub Actions workflows and repository governance engine |

Additional project repositories should be added to this table when they are
added as submodules.

## Actions and workspace workflows

[`actions`](actions) is shared infrastructure for the Blink Labs organization,
not a normal application dependency. It provides reusable `workflow_call`
workflows for testing, linting, publishing, version checks, and Conventional
Commit validation.

It also contains the governance engine and its `repos-config.yaml`. That file
is the operational source of truth for the repositories it manages: the sync
workflow reconciles repository settings, collaborators, branch protection, and
generated workflow wrappers, writing changes directly to each target's default
branch. The `actions` repository manages other repositories and is intentionally
not part of its own managed set.

The submodule here gives this workspace a reproducible checkout for inspecting
and developing the shared workflows and governance code. It does not pin the
workflow version consumed by downstream repositories: those workflow wrappers
currently reference paths such as
`blinklabs-io/actions/.github/workflows/reuseable-go-test.yml@main`. Changes to
the `actions` repository can therefore affect consumers independently of this
monorepo's submodule pointer.

## Adding a repository

From the monorepo root, add a repository at a descriptive path:

```sh
git submodule add git@github.com:blinklabs-io/<repository>.git repos/<repository>
git submodule update --init --recursive
```

Before adding a repository, confirm its canonical URL and intended local path.
Read its `README.md`, `AGENTS.md`, `CONTRIBUTING.md`, and other repository-level
guidance before making changes inside it.

## Updating repositories

To initialize or restore all repositories at the revisions recorded by the
monorepo:

```sh
git submodule update --init --recursive
```

To update one repository intentionally, enter its directory, fetch or check out
the desired commit, then review and commit the changed submodule pointer in the
parent repository:

```sh
cd repos/<repository>
git fetch origin
git checkout <commit-or-ref>
cd ../..
git diff --submodule=log -- repos/<repository>
git add repos/<repository>
git commit -s -m "build: update <repository> submodule"
```

Use a Conventional Commit type that describes the reason for the update when a
more specific type is appropriate. Keep the parent pointer update separate from
source changes made in the nested repository.

## Working across repository boundaries

Changes to a project's source belong in that project's repository. The parent
monorepo should contain the pinned submodule reference and shared workspace
documentation or automation. Validate a project using its own documented
commands before updating its pointer here.

For organization-wide policies and community defaults, see
[`repos/.github/CONTRIBUTING.md`](.github/CONTRIBUTING.md) and
[`repos/.github/SECURITY.md`](.github/SECURITY.md).
