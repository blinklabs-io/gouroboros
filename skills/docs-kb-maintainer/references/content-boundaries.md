# Documentation boundaries

| Destination | Use for | Preserve |
| --- | --- | --- |
| `repos/docs` | Public installation, configuration, quickstarts, concepts, and operations | MDX structure, navigation metadata, links, package-manager/build contract |
| `repos/kb` | Deep engineering training, architecture, protocol, testing, debugging, and contribution context | Numbered books, each README, `00-start-here.md`, glossary, source map, pinned public sources |
| Project README/docs | Repository-local commands, API details, architecture, and release facts | Local source of truth and version-specific instructions |
| Parent `docs/` | Cross-repository patterns and workspace policy | Links to owning repositories; no duplicated large explanations |

Use issues for durable follow-up work. Do not commit plans or agent scratch
notes. If a claim can drift, link to the source file, workflow, issue, or
pinned public commit that owns it.
