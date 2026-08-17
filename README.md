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
`AGENTS.md` or equivalent contributor documentation inside the project you are
working on. A submodule's local instructions take precedence for work within
that submodule.

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

## Planned layout

The exact layout will evolve as projects and tooling are added. The intended
top-level organization is:

```text
.
├── skills/       # Reusable agent skills
├── plugins/      # Agent plugins and plugin metadata
├── docs/         # Shared documentation and design notes
├── scripts/      # Workspace-level development and maintenance scripts
└── repos/        # Blink Labs repositories tracked as Git submodules
    └── .github/  # Organization-wide contribution and security defaults
```

Directories may be introduced incrementally; their presence is not required
for a checkout to be useful.

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
