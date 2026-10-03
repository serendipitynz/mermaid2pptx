---
id: TASK-4
title: >-
  Publish releases as a draft first so the workflow works with immutable
  releases
status: To Do
assignee: []
created_date: '2026-10-03 21:31'
labels: []
milestone: m-4
dependencies: []
references:
  - .github/workflows/release.yml
ordinal: 4000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
GitHub's immutable releases setting locks a release's assets and tag the moment the release is published: nothing can be added, replaced or deleted afterwards, and the tag cannot be moved or reused. Release notes stay editable. It protects whoever downloads a pinned version, `SHA256SUMS.txt` included, from an asset being swapped under the same URL. The owner wants it on for every repository that publishes releases. serenebach already has it, and mallow / backlog-atlas can take it as they are, because they upload into a draft and publish last.

This repository cannot take it yet. `.github/workflows/release.yml` publishes first and uploads afterwards: line 53 runs `gh release create` without `--draft` when the pushed tag has no release, which publishes it with no assets, and line 54 then runs `gh release upload dist/* --clobber` into it. With the setting on, the release is immutable from line 53 onward, so the upload fails and the version ships with no binaries. The `gh release view || create` reuse path breaks the same way: a published release can no longer take uploads, and `--clobber` can no longer replace an asset.

The workflow has to build first and publish last. Either `gh release create` takes the assets in one call (the gh CLI then creates a draft, uploads into it and publishes it), or the workflow creates a draft explicitly, uploads, and publishes with `gh release edit --draft=false`. Settle which one at the start of the task. Reruns matter too: a rerun after a failed upload may reuse a leftover draft, but must stop when the tag already has a published release. backlog-atlas's `release.yml` (Create (or reuse) the draft release) is a working example of that check.

Turning the setting on is a repository setting, so the owner does it. It applies only to releases published after it is enabled; v0.1.0–v0.3.0 stay mutable.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 release.yml builds every asset (archives and SHA256SUMS.txt) before the release is published, and publishes it only after all of them are uploaded
- [ ] #2 A rerun for a tag whose release is still a draft reuses that draft; a rerun for a tag whose release is already published stops with an error instead of trying to upload
- [ ] #3 Immutable releases is enabled on the repository (owner action) before the next release is tagged
- [ ] #4 The next release (v0.4.0) goes out through the new workflow with all assets attached, and gh release view reports isImmutable=true for it
<!-- AC:END -->
