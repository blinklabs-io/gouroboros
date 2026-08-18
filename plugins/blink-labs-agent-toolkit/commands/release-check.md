---
description: "Review a Blink Labs Docker image or release workflow from Dockerfile through multi-architecture manifest, tags, and registry publication"
argument-hint: "[repository path or image name]"
allowed-tools: ["Bash", "Glob", "Grep", "Read"]
---

# Blink Labs release check

Target: "$ARGUMENTS".

Follow the `docker-release-reviewer` skill and its `references/release-checklist.md`.

## Steps

1. Trace the image end to end: base images and build arguments, per-architecture
   build jobs, manifest creation, tag triggers, release tags, and `latest`.
2. Check image provenance. Prefer an equivalent `blinklabs-io` image; if an
   upstream or third-party image is used, record why, how its tag or digest is
   verified, and the migration path back.
3. Inspect permissions, secrets, provenance attestations, and the separation
   between CI builds and publishing jobs.
4. Compare the repository's `.github/workflows/` against its `repos/actions`
   profile. Generated wrappers are outputs; change the profile or the reusable
   workflow when the wrapper is wrong.
5. Run `docker build --check .`, `actionlint` on changed workflows, and any
   native repository target. Full multi-stage or Haskell builds are expensive —
   state clearly when one was skipped.

## Constraints

Never push an image, manifest, or tag, and never mutate registry state during a
review without explicit authorization. A successful single-architecture build is
not evidence of a valid multi-architecture manifest; say so rather than
implying coverage you do not have.

## Report

Cover: base image provenance, architecture coverage, tag and manifest
correctness, permissions and secrets, workflow-generation drift, and the checks
that were skipped with reasons.
