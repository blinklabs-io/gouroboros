# Agent toolkit installation

The Blink Labs Agent Toolkit packages the shared repository-maintenance and
review skills for both Codex and Claude Code. Install only from a trusted
checkout of this repository; plugins are trusted code and may add executable
hooks, tools, or other capabilities in the future.

## Codex

From the root of a clone, add the local marketplace and install the toolkit:

```sh
codex plugin marketplace add .
codex plugin add blink-labs-agent-toolkit@blink-labs-team
```

To share a checked-out version with teammates, provide the repository path or
the Git revision they should clone. The Codex marketplace metadata is in
`.agents/plugins/marketplace.json`.

## Claude Code

Claude Code uses the Claude marketplace metadata in `.claude-plugin/`:

```sh
claude plugin marketplace add . --scope user
claude plugin install blink-labs-agent-toolkit@blink-labs-team
```

Use `--scope project` when the installation should be recorded for everyone
working in the current project, or `--scope local` for a private installation.
After installing or updating a plugin in an interactive session, run
`/reload-plugins`.

For a local development checkout without installing it, load the plugin for one
session:

```sh
claude --plugin-dir ./plugins/blink-labs-agent-toolkit
```

The skills are namespaced in Claude Code, for example:
`/blink-labs-agent-toolkit:blink-repo-maintainer`. Claude Code also invokes
skills automatically when their descriptions match the task.

## Direct, tool-neutral use

Both clients can use the source skill files directly. Read the relevant
`skills/<name>/SKILL.md` or the packaged copy under
`plugins/blink-labs-agent-toolkit/skills/<name>/SKILL.md`, together with the
target repository's `AGENTS.md` and `CLAUDE.md`. The skill instructions do not
depend on Codex-only tools; `.codex-plugin/` and `.claude-plugin/` contain only
client distribution metadata.

## Updating

Update the parent checkout, refresh the marketplace, and reinstall or reload
the plugin:

```sh
git pull --ff-only
git submodule update --init --recursive
claude plugin marketplace update blink-labs-team
codex plugin marketplace update blink-labs-team
```

If a plugin is installed at project scope, review the resulting
`.claude/settings.json` change as normal repository configuration. Do not commit
personal or ephemeral plan files.
