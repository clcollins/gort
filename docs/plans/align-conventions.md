# Plan — Align Repository with CONVENTIONS.md

## Problem

CONVENTIONS.md was added from a standard gist and several areas of the repository
do not comply with the conventions it describes. Additionally, some conventions
needed minor updates to accurately reflect GORT's hybrid CI approach (Go checks
on native runners, lint checks in a CI container).

## Changes

### Repository compliance fixes

- Add `.containerignore` for build context exclusion
- Pin Containerfile base image tags (go-toolset:1.25, ubi-minimal:9.6)
- Set `ENFORCE=1` in `check-containerfile-tags.sh`
- Create `test/Containerfile.ci` (fedora-minimal:42 with all lint tools)
- Add `ci-build`, `ci-checks` Makefile targets; update `ci-all` to use them
- Update GHA CI workflow to build CI container and run lint jobs inside it
- Remove superseded `hack/ci-container.sh`

### CONVENTIONS.md updates

- CI Testing: document hybrid approach (CI container for lints, native for Go)
- Makefile Standards: remove `test` should run `ci-all` requirement
- Plan Documents: allow both descriptive and numbered filenames
