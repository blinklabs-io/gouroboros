# Module audit rules

Use:

```sh
rg --files repos -g 'go.mod' -g '!**/.git/**' | sort
rg -n --glob 'go.mod' --glob '!**/.git/**' \
  '^(module|go |toolchain|replace)|github.com/blinklabs-io/' repos
```

For each module record:

1. module path and Go/toolchain floor;
2. direct and indirect Blink dependencies;
3. active and commented local replacements;
4. nested-module boundary and owning repository;
5. matching workspace submodule or external canonical source;
6. module-path and repository-URL consistency.

Normalize semantic-major suffixes when mapping paths (`/v2` remains part of
the module identity, not the checkout directory). Treat generated OpenAPI,
UI, example, and Antithesis modules independently.

Report missing or suspicious entries as a table with module, requiring file,
expected source, current version, and recommended action. Do not silently add a
fork, replacement, or submodule to make the graph appear complete.
