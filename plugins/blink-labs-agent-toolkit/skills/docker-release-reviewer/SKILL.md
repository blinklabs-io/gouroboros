---
name: docker-release-reviewer
description: Review Blink Labs Dockerfiles, image provenance, multi-architecture builds, manifest tags, publishing workflows, and generated GitHub Actions wrappers. Use when changing Docker images, release workflows, compose images, registry tags, or actions governance configuration.
---

# Docker Release Reviewer

Use this skill for Docker build or release changes. Read the
[release checklist](references/release-checklist.md), the target repository's
Docker and contribution guidance, and the corresponding `repos/actions`
profile before forming a review.

## Workflow

1. Trace the image from Dockerfile/build arguments through architecture jobs,
   manifest creation, registry publication, release tags, and `latest`.
2. Prefer an equivalent `blinklabs-io` image. If an upstream or third-party
   image is necessary, record why, how its tag is verified, and its migration
   path.
3. Inspect architecture support, tag triggers, permissions, provenance
   attestations, secrets, and the distinction between CI and publishing.
4. Treat generated workflow wrappers as outputs of `repos/actions`; change the
   profile or reusable workflow source when that is the requested scope.
   Verify each `uses: blinklabs-io/actions/...@ref` path exists at the current
   source ref. Pin release and secret-bearing reusable workflows to full
   commit SHAs and keep caller permissions explicit and least-privileged.
   Verify each `uses: blinklabs-io/actions/...@ref` path exists at the current
   source ref. Pin release and secret-bearing reusable workflows to full
   commit SHAs and keep caller permissions explicit and least-privileged.
5. Run `docker build --check .`, `actionlint`, and native repository checks when
   available. Full builds, registry pushes, and live deployment checks require
   explicit authorization and should be reported when skipped.

Never infer that a successful single-architecture build proves a valid release
manifest. Never push images or mutate registry state during a review without
explicit authorization.
