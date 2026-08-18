---
name: docs-kb-maintainer
description: Maintain Blink Labs public documentation and the engineering knowledge base with correct content boundaries, navigation, source links, Markdown structure, and reproducible references. Use when changing repos/docs-site, repos/kb, shared project documentation, onboarding material, or generated documentation pointers.
---

# Docs and Knowledge Base Maintainer

Use this skill when documentation is part of a cross-repository change. Read
the [content boundaries reference](references/content-boundaries.md), then
the local instructions in `repos/docs-site` or `repos/kb`.

## Workflow

1. Choose the destination: public `docs` for concise product, installation,
   configuration, quickstart, and operations content; `kb` for durable deep
   technical learning, architecture, protocol, testing, and contribution
   context.
2. Inspect navigation metadata, book structure, glossary, source map, and
   existing cross-links before adding content.
3. Prefer links to the owning README, source file, issue, specification, or
   pinned public commit over copied explanations. Keep examples reproducible.
4. Preserve existing terminology, headings, front matter, and link style.
   Update navigation metadata whenever a public page is added or moved.
5. For `repos/docs-site` (the active `blinklabs-io/docs` checkout), run
   `npm ci`, `npm run check`, and `npm run build` when available. Content lives
   under `src/content/docs/` and uses Astro/Starlight navigation. For the
   knowledge base, validate Markdown structure and links; do not invent a
   build system it does not have.
6. Report stale claims, missing source links, and unresolved cross-repository
   documentation as issues rather than hiding them in a plan file.

Plans remain local and ephemeral. Durable documentation work belongs in the
appropriate repository and, when follow-up is needed, a repository issue.
