---
id: TASK-5
title: Build with Go 1.27.3 and scan with govulncheck so GO-2026-6088 stops shipping
status: In Review
assignee: []
created_date: '2026-10-25 11:24'
updated_date: '2026-10-25 11:28'
labels: []
milestone: m-4
dependencies: []
references:
  - go.mod
  - .github/workflows/ci.yml
  - .github/workflows/release.yml
  - internal/convert/svg.go
modified_files:
  - go.mod
  - .github/workflows/govulncheck.yml
ordinal: 5000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
go.mod carries only `go 1.24`, with no `toolchain` line, and both ci.yml and release.yml install Go through `setup-go` with `go-version-file: go.mod`. They therefore build with the newest 1.24 patch, go1.24.13, a line that no longer receives security fixes. Scanned with that toolchain, govulncheck reports GO-2026-6088 (encoding/xml has no recursion depth guard during decode, fixed in go1.25.13) as reachable on linux, windows and darwin: `internal/convert/svg.go:53` `parseXMLTree` calls `xml.Decoder.Token`. An SVG nested deeply enough can crash the converter. v0.3.0 was released through this workflow, so its binaries most likely carry it.

With go1.27.3 the scan is clean on all three platforms. GO-2026-6809 (crypto/x509), which serenebach and snz-studio just fixed, is not reachable here because the tool makes no TLS connection.

Nothing checked for this automatically; vulnerabilities in Go were only found by a manual audit. serenebach's `.github/workflows/govulncheck.yml` (PR #163) scans on every PR, on push to main and weekly, once per released target, and fails only on Symbol Results. The same shape fits here, with the matrix narrowed to the five targets release.yml ships.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 go.mod has toolchain go1.27.3, and ci.yml / release.yml build with it through setup-go
- [x] #2 A govulncheck workflow runs on PRs, on push to main, weekly and on demand, for every target release.yml ships, and fails only on Symbol Results
- [x] #3 govulncheck reports no Symbol Results for those targets with go1.27.3
- [x] #4 gofmt -l, go vet ./... and go test ./... pass
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
- go.mod gains `toolchain go1.27.3` via `go mod edit -toolchain`; the `go 1.24` directive stays. It is the minimum version for building from source, and nothing in the code needs a newer language version, so raising it would only refuse older local toolchains without fixing anything — the toolchain line is what selects the compiler. setup-go v6 reads the `toolchain` directive when present (its README: "If the toolchain directive is present, its version is used"), so ci.yml and release.yml need no change. #1 stays open until the PR's CI shows go1.27.3 in the setup-go step.
- `.github/workflows/govulncheck.yml` follows serenebach's (PR #163): its own workflow so the weekly cron re-runs only the scan, fails only on Symbol Results, govulncheck pinned to v1.9.0 and installed before GOOS/GOARCH are set. The matrix is the five targets release.yml ships (darwin amd64/arm64, linux amd64/arm64, windows amd64) instead of serenebach's eight. `cache: false` matches ci.yml, since there is no go.sum.
- Local checks (macOS arm64, Go auto-switched to go1.27.3 from go.mod): `gofmt -l .` prints nothing, `go vet ./...` and `go test ./...` pass, the windows/amd64 cross-build passes. govulncheck v1.9.0 with CGO_ENABLED=0 reports No vulnerabilities found for each of the five targets, against the go1.27.3 standard library. With GOTOOLCHAIN=go1.24.13 the same scan reports GO-2026-6088 reachable through `internal/convert/svg.go:53`.
- The fixed binaries reach users only with a new release; v0.3.0 is affected. Whether that is a v0.3.1 patch or waits for v0.4.0 (m-4) is the owner's call.
<!-- SECTION:NOTES:END -->
