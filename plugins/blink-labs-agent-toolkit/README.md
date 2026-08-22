# Blink Labs Agent Toolkit

Shared agentic-coding assets for the Blink Labs `clanker` workspace: skills,
slash commands, review subagents, and workspace guards for Cardano, Go, Docker,
documentation, infrastructure-as-code maintenance, and live operations.

This directory is the canonical source. The workspace `skills/` directory and
the two guides under `docs/` are symlinks into it, so there is exactly one copy
of every file.

## Contents

```
.claude-plugin/plugin.json   Claude Code manifest
.codex-plugin/plugin.json    Codex manifest and UI metadata
commands/                    slash commands (/orient, /validate, /review, …)
agents/                      review and audit subagents
hooks/                       commit, boundary, and session guards
references/                  guides shared by several skills
skills/<name>/SKILL.md       the skills themselves
```

The full catalog, including when each piece applies, is in
[`docs/skill-catalog.md`](../../docs/skill-catalog.md).

## Install

Run these commands from the root of the `clanker` checkout:

```sh
# Claude Code
claude plugin marketplace add .
claude plugin install blink-labs-agent-toolkit@blink-labs-team
```

For a development checkout without installing:

```sh
claude --plugin-dir ./plugins/blink-labs-agent-toolkit
```

Codex uses `.agents/plugins/marketplace.json` at the workspace root:

```sh
codex plugin marketplace add .
codex plugin add blink-labs-agent-toolkit@blink-labs-team
```

Claude Code reloads an installed update with `/reload-plugins`. Start a new
Codex session after installing or updating the plugin. See
[`docs/agent-toolkit-installation.md`](../../docs/agent-toolkit-installation.md)
for scopes, updates, and tool-neutral use.

## Guards

The hooks enforce organization policy that is otherwise easy to miss:

- `git commit` is denied without DCO sign-off and a Conventional Commit subject
  (`BLINK_SKIP_COMMIT_GUARD=1` to bypass);
- editing under `repos/` emits a one-time submodule boundary notice
  (`BLINK_SKIP_BOUNDARY_NOTICE=1`);
- a new session is briefed on uninitialized submodules and moved pointers
  (`BLINK_SKIP_WORKSPACE_BRIEF=1`).

Every guard fails open — bad input never blocks work.

## Changing the toolkit

Read the `agent-toolkit-authoring` skill, then validate:

```sh
make validate
```

Bump `version` in both manifests when contents change, and update
`docs/skill-catalog.md` when adding a skill, command, or subagent.
