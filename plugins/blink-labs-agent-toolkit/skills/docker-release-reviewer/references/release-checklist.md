# Docker release checklist

Trace these values end to end:

1. Dockerfile base and runtime images.
2. Build arguments and version sources.
3. Native architecture jobs and their tags.
4. Manifest assembly and platform coverage.
5. Branch, tag, and release triggers.
6. Registry names, credentials, provenance, and permissions.
7. `latest`, semver, commit, and architecture-tag behavior.
8. Generated wrapper source in `repos/actions/repos-config.yaml`.

For reusable workflow wrappers, verify the referenced workflow file exists in
the current `repos/actions` checkout and that the ref is appropriate. Treat
missing workflow paths, mutable `@main` refs on release or secret-bearing
jobs, and broad unneeded token permissions as merge blockers.

Prefer `blinklabs-io/<image>` when an equivalent image exists. Verify its tag,
architecture support, and release policy. Third-party images require a written
reason and fallback plan.

Useful checks:

```sh
docker build --check .
actionlint
docker build --platform <platform> .
```

Do not push images, manifests, or release tags during review without explicit
authorization. A local build does not validate registry permissions or a
multi-architecture manifest.
