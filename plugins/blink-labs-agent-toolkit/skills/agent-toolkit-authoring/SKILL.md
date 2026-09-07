---
name: agent-toolkit-authoring
description: Add or change skills, slash commands, subagents, and hooks in the Blink Labs agent toolkit so they work in Claude Code and stay usable from Codex. Use when writing a new SKILL.md, editing an existing one, adding a command or subagent, changing hooks, or updating plugin and marketplace metadata in the clanker workspace.
---

# Agent Toolkit Authoring

Use this skill when changing the toolkit itself. The toolkit is the entry point
for agentic work across Blink Labs, so a broken skill file is a workspace-wide
outage, not a local annoyance.

## Layout

```
plugins/blink-labs-agent-toolkit/     canonical plugin (single source of truth)
├── .claude-plugin/plugin.json        Claude Code manifest
├── .codex-plugin/plugin.json         Codex manifest and UI metadata
├── commands/<name>.md                slash commands
├── agents/<name>.md                  subagents
├── hooks/hooks.json + *.py           workspace guards
├── references/                       shared guides used by several skills
└── skills/<name>/
    ├── SKILL.md                      the skill itself
    ├── references/*.md               detail loaded on demand
    └── agents/openai.yaml            optional Codex UI metadata

skills -> plugins/blink-labs-agent-toolkit/skills     (symlink)
docs/go-repository-guide.md -> ../plugins/.../references/...   (symlink)
```

`skills/` and the two `docs/` guides are symlinks into the plugin. Edit the
files under `plugins/blink-labs-agent-toolkit/`; never replace a symlink with a
copy, because a second copy will silently drift.

## Writing a skill

1. Front matter needs exactly two keys for a model-invoked skill: `name`
   (kebab-case, identical to the directory name) and `description`.
2. The description is the routing signal. Write what the skill covers *and* when
   to use it, naming the concrete repositories, file types, and tasks that
   should trigger it. A vague description means the skill never loads.
3. Keep `SKILL.md` short enough to read in full. Move checklists, tables, and
   long detail into `references/` and link them, so they load only when needed.
4. Write rules an agent can act on. "Run `make import-boundaries` after changing
   package structure" is useful; "follow best practices" is not.
5. State the constraints explicitly — what not to run, what not to claim, what
   requires authorization. Most toolkit value is in the prohibitions.
6. Link related skills with a relative path (`../other-skill/SKILL.md`). Use
   `references/<file>.md` for a skill's own detail, and
   `../<skill>/references/<file>.md` for another skill's. Do not link upward out
   of the plugin — those paths break once the plugin is installed standalone.
7. Add `agents/openai.yaml` with `display_name`, `short_description`, and
   `default_prompt` so the skill also presents correctly in Codex.

## Writing a command or subagent

- Commands live in `commands/<name>.md` with `description`, optional
  `argument-hint`, and optional `allowed-tools`. Reference `$ARGUMENTS`, define
  the steps, and end with the expected report shape.
- Subagents live in `agents/<name>.md` with `name`, `description`, `tools`,
  `model`, and `color`. The description decides when the agent is dispatched;
  the body is its whole operating context, so restate the constraints rather
  than assuming the parent session's rules carry over.
- Read-only agents must be given read-only tool sets and told not to mutate
  state — the tool list is the enforcement, the prose is the intent.

## Hooks

Hooks execute on the user's machine. Keep them cheap, deterministic, and
non-blocking unless the rule is a hard organization policy. Every guard needs an
escape-hatch environment variable, and every guard must fail open: an
unparseable input exits zero rather than blocking work.

## Validate before finishing

```sh
make validate          # or: scripts/validate-toolkit.sh
```

That checks JSON validity, front-matter presence, `name`-to-directory match,
symlink integrity, hook syntax, and internal link resolution. Run it for any
change under `plugins/`, `skills/`, or `docs/`.

## Keep the documentation in step

A new skill, command, or subagent must also appear in `docs/skill-catalog.md`,
and in `AGENTS.md` or `README.md` when it changes how work starts.

When the toolkit's contents change, bump the plugin `version` in all three
places that declare it — `.claude-plugin/plugin.json`, `.codex-plugin/plugin.json`,
and the plugin's entry in the workspace `.claude-plugin/marketplace.json`.
`make validate` fails when they disagree. The marketplace entry is the one
that is easy to miss, and it is the one Claude Code resolves an installed
version from.
