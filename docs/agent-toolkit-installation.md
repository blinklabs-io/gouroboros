# Agent toolkit installation

The Blink Labs Agent Toolkit packages the shared skills, slash commands, review
subagents, and workspace guards for both Claude Code and Codex. Install only
from a trusted checkout of this repository: plugins are trusted code, and this
one ships executable hooks.

The canonical source is `plugins/blink-labs-agent-toolkit/`. The workspace
`skills/` directory and the `docs/go-repository-guide.md` and
`docs/repository-patterns.md` files are symlinks into it, so a `git clone`
without symlink support will not give you a working toolkit.

## Claude Code

Claude Code reads the marketplace metadata in `.claude-plugin/`:

```sh
claude plugin marketplace add . --scope user
claude plugin install blink-labs-agent-toolkit@blink-labs-team
```

Use `--scope project` when the installation should be recorded for everyone
working in this repository, or `--scope local` for a private installation. After
installing or updating a plugin in an interactive session, run
`/reload-plugins`.

This repository already registers the marketplace and enables the plugin in
`.claude/settings.json`, so a fresh clone picks it up without any manual step.
That file also carries a read-only permission allowlist for the workspace's
usual inspection and validation commands, sends pushes and pull-request
mutations to a prompt, and denies destructive registry, cluster, and Terraform
operations along with reads of key material. Personal overrides belong in
`.claude/settings.local.json`, which is ignored by Git.

For a local development checkout without installing anything:

```sh
claude --plugin-dir ./plugins/blink-labs-agent-toolkit
```

Skills load automatically when a task matches their description. The explicit
forms are namespaced, for example
`/blink-labs-agent-toolkit:blink-repo-maintainer`. Slash commands appear as
`/orient`, `/validate`, `/review`, `/dep-audit`, `/release-check`, and
`/submodule-sync`. Subagents are dispatched by name, for example
`blink-protocol-auditor`.

### Guards

Installing the plugin activates three hooks. Each one fails open and each has an
escape hatch:

| Guard | Effect | Bypass |
|---|---|---|
| Commit policy | Denies `git commit` without DCO sign-off or a Conventional Commit subject; warns when a plan file is staged | `BLINK_SKIP_COMMIT_GUARD=1` |
| Submodule boundary | One notice per submodule per session when editing under `repos/` | `BLINK_SKIP_BOUNDARY_NOTICE=1` |
| Workspace brief | Reports uninitialized submodules and moved pointers at session start | `BLINK_SKIP_WORKSPACE_BRIEF=1` |

## Codex

From the root of a clone, add the local marketplace and install the toolkit:

```sh
codex plugin marketplace add .
codex plugin add blink-labs-agent-toolkit@blink-labs-team
```

The Codex marketplace metadata is in `.agents/plugins/marketplace.json`. Codex
uses the same `SKILL.md` files; the commands, subagents, and hooks are Claude
Code features and are simply unused there.

## Direct, tool-neutral use

Both clients can read the source files directly without installing anything.
Read the relevant `skills/<name>/SKILL.md` together with the target repository's
`AGENTS.md` and `CLAUDE.md`. The skill instructions do not depend on
client-specific tools.

## Updating

```sh
git pull --ff-only
git submodule update --init --recursive
claude plugin marketplace update blink-labs-team
codex plugin marketplace update blink-labs-team
```

Then `/reload-plugins` in an interactive Claude Code session.

If a plugin is installed at project scope, review the resulting
`.claude/settings.json` change as ordinary repository configuration. Never
commit personal or ephemeral plan files.

## Verifying a toolkit change

```sh
make validate
```

This checks manifest JSON, skill front matter, command and subagent metadata,
hook syntax and behavior, symlink integrity, and internal link resolution. Run
it before committing anything under `plugins/`, `skills/`, or `docs/`.
