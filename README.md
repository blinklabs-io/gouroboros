# Blink Labs Monorepo

This repository is the home for the Blink Labs engineering workspace. It brings
together the shared agentic-coding tooling and the source repositories that make
up the Blink Labs ecosystem.

The monorepo is intended to provide a common place for:

- Git submodules pointing to Blink Labs projects
- Reusable skills, plugins, prompts, and other agent tooling
- Documentation and conventions shared across projects
- Workspace-level scripts for developing and maintaining the collection

Individual projects remain independently versioned repositories. Unless a
document says otherwise, changes to a project should be made in that project's
own repository and then reflected here by updating its submodule reference.

## Getting started

Clone the workspace and initialize all nested repositories:

```sh
git clone <repository-url>
cd clanker
git submodule update --init --recursive
```

After cloning, read the repository-level [AGENTS.md](AGENTS.md) and any
`CLAUDE.md` or equivalent contributor documentation inside the project you are
working on. A submodule's local instructions take precedence for work within
that submodule. For Go work and cross-repository reviews, start with the
[common-ground guide](docs/go-repository-guide.md).

## Agent toolkit

Shared agentic-coding assets live in `plugins/blink-labs-agent-toolkit/` and
cover Cardano, Go, Docker, documentation, and infrastructure work across every
project in the workspace:

- **Skills** that load automatically when a task matches them, from protocol and
  application review to dependency auditing and validation discipline.
- **Slash commands** — `/orient`, `/validate`, `/review`, `/dep-audit`,
  `/release-check`, `/submodule-sync`.
- **Subagents** for protocol, application, module, release, and validation
  review, dispatchable in parallel across repositories.
- **Workspace guards** that enforce DCO sign-off and Conventional Commits, warn
  on submodule boundary crossings, and brief a new session on workspace state.
- **Workspace scripts** for repeatable maintenance, including `make scan-prs`
  for current-head pull-request review, direct and team review requests, and
  merge-gate scans.

`.claude/settings.json` registers and enables both the Blink Labs toolkit and
the Cardano Foundation's
[Cardano Dev Skills](https://github.com/cardano-foundation/cardano-dev-skills),
so Claude Code adopts both plugins automatically in a fresh clone. The complete
Blink Labs catalog is in [docs/skill-catalog.md](docs/skill-catalog.md), and
Claude Code and Codex installation, scopes, and guard bypasses are in the
[agent toolkit installation guide](docs/agent-toolkit-installation.md).

### Install

From the root of this checkout, register the local marketplace and install the
toolkit in the client you use:

```sh
# Claude Code
claude plugin marketplace add . --scope user
claude plugin install blink-labs-agent-toolkit@blink-labs-team
claude plugin marketplace add cardano-foundation/cardano-dev-skills --scope user
claude plugin install cardano-dev-skills@cardano-dev-skills

# Codex
codex plugin marketplace add .
codex plugin add blink-labs-agent-toolkit@blink-labs-team
```

Claude Code can reload an installed update with `/reload-plugins`. Start a new
Codex session after installing or updating the plugin. Use
`claude --plugin-dir ./plugins/blink-labs-agent-toolkit` for a Claude Code
development checkout without installing it.

Validate any change to the toolkit with:

```sh
make validate
```

Scan organization pull requests without changing GitHub state:

```sh
make scan-prs
```

See [pull request operations](docs/pr-operations.md) for filters and JSON
output.

## Contributing

The organization-wide contribution guidance is checked out in
[`repos/.github/CONTRIBUTING.md`](repos/.github/CONTRIBUTING.md), with the
corresponding [security policy](repos/.github/SECURITY.md). Read those files
before contributing to this workspace or any of its projects.

At the workspace level:

- Use [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/)
  for every commit.
- Sign off every commit for the [Developer Certificate of Origin](https://developercertificate.org/)
  with `git commit -s` or `git commit --signoff`.
- Check the applicable `CODEOWNERS` file and project-specific contribution
  instructions before opening a pull request.

## Repository layout

The workspace is organized as follows; it will keep evolving as projects and
tooling are added:

```text
.
├── .claude/      # Workspace Claude Code settings (marketplace, permissions)
├── skills/       # Symlink into the toolkit's skills
├── plugins/      # Agent plugins; the toolkit is the canonical source
├── docs/         # Shared documentation and the toolkit catalog
├── scripts/      # Workspace-level development and maintenance scripts
└── repos/        # Blink Labs repositories tracked as Git submodules
    └── .github/  # Organization-wide contribution and security defaults
```

`skills/`, `docs/go-repository-guide.md`, and `docs/repository-patterns.md` are
symlinks into `plugins/blink-labs-agent-toolkit/`, which holds the only copy of
each file. Edit the files under `plugins/`; never replace a symlink with a copy.

The current repository families, validation matrix, and governance findings are
documented in [docs/repository-patterns.md](docs/repository-patterns.md). For Go
work and cross-repository code reviews, use the [Go repository common-ground
guide](docs/go-repository-guide.md). Every skill, command, subagent, and guard
is listed in [docs/skill-catalog.md](docs/skill-catalog.md).

## Working with submodules

Submodules are pinned intentionally so that a workspace checkout is reproducible.
When changing a submodule, commit and push the change in the nested repository
first, then commit the updated submodule pointer in this repository.

To bring the workspace to the revisions recorded by this repository:

```sh
git submodule update --init --recursive
```

To inspect the current submodule state:

```sh
git submodule status --recursive
```

## License

Unless noted otherwise, this repository is distributed under the [MIT
License](LICENSE). Individual submodules and tooling may have their own
licenses and contribution requirements.
