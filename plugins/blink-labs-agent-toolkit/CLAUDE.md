# Blink Labs Agent Toolkit

This plugin packages the shared Blink Labs `SKILL.md` files for distribution to
Claude and Codex users. The skills are tool-neutral; Claude should read the
relevant file under `skills/` directly when a task matches its description.

The `.codex-plugin/plugin.json` and `agents/openai.yaml` files provide Codex
distribution and UI metadata only. They are not required to use these skills
from Claude.

Use the same repository boundaries, upstream-only dependency policy, review-bot
sequence, mandatory human review, and reviewer re-request workflow documented
in the packaged skills and the workspace root `CLAUDE.md`.
