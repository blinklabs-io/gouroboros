# Blink Labs Agent Toolkit

This plugin is the canonical source for Blink Labs agentic-coding assets. The
workspace `skills/` directory symlinks here; do not create a second copy of any
file.

Claude Code loads `skills/`, `commands/`, `agents/`, and `hooks/` from this
directory automatically. The `.codex-plugin/plugin.json` and
`skills/*/agents/openai.yaml` files are Codex distribution and UI metadata only;
they are not needed to use anything here from Claude.

When changing this plugin, use the `agent-toolkit-authoring` skill and run
`make validate` from the workspace root. Keep relative links inside the plugin —
a link that escapes this directory breaks once the plugin is installed
standalone, and `make validate` fails on it.

The repository boundaries, upstream-only dependency policy, bot-first review
sequence, mandatory human review, reviewer re-request workflow, and commit
message scope documented in the workspace root `AGENTS.md` and `CLAUDE.md` apply
to work here as well.
