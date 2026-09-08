---
id: TASK-1
title: Track PowerPoint render-export and connector-following AppleScript in the repo
status: To Do
assignee: []
created_date: '2026-09-08 21:28'
labels: []
milestone: m-3
dependencies: []
ordinal: 1000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Verifying generated decks needs PowerPoint's own rendering engine. Structural XML checks miss two classes of defect that only appear once PowerPoint lays the slide out: labels it re-wraps, and shapes whose coordinates collapse. A working batch pptx-to-PDF exporter already exists, but it lives outside version control as a compiled .scpt, so it cannot be reviewed, diffed, or re-used by anyone else.

Two capabilities are wanted:

1. Batch render export. Walk a folder of .pptx, export each one to PDF through PowerPoint, skip PowerPoint's ~$ temp files and already-exported PDFs, and report converted/skipped/failed counts.

2. Connector-following test. Open a .pptx, move one node shape by a fixed offset, save, re-export to PDF, and report whether the connector endpoints moved with the shape. This is the only way to confirm that stCxn/endCxn actually binds in PowerPoint: a static PDF or PNG shows the shapes but never shows whether the line follows. The claim that connectors stay attached is currently unverified against the application itself.

Store the script as plain-text .applescript rather than a compiled .scpt, so that changes are reviewable in a diff. osascript runs plain text directly, and osacompile can produce a .scpt when a double-clickable droplet is wanted. An existing compiled script can be recovered with osadecompile.

The repo has no scripts directory yet; this would create one.

This must stay an optional manual step. It requires macOS plus Microsoft PowerPoint, so it cannot become a dependency of go test, which has to keep running on any platform with only the Go toolchain.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 A plain-text AppleScript under scripts/ exports every .pptx under a given folder to a PDF beside the source, skipping ~$ temp files and existing PDFs, and reports converted/skipped/failed counts
- [ ] #2 A connector-following mode moves a chosen shape in a .pptx by a given offset, saves a copy, re-exports the PDF, and reports each connector endpoint's position before and after the move
- [ ] #3 Both run via osascript from the repo root with no compilation step
- [ ] #4 Neither script is reachable from go test; go test still passes with no macOS or PowerPoint present
- [ ] #5 AGENTS.md documents the scripts as an optional manual verification step, alongside the existing python-pptx inspection notes
- [ ] #6 The connector-following result for a flowchart sample is recorded: whether the line endpoints move with the node in PowerPoint
<!-- AC:END -->
