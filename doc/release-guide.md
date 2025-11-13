# Preparing a new Release of Gorums

This repository includes a `Makefile` with helpers that automate the repetitive parts of preparing a release.
The recommended flow below uses those targets.

## Makefile helper targets

Useful `make` targets provided in the repository:

- `make release-tools` — install/check `gorelease` and `gh`.
- `make prepare-release` — regenerates protos (via `genproto`), tidies modules, checks important tool versions and runs `gorelease` to suggest a version. After a suggested version is shown it prints instructions for editing version constants and re-running `make genproto` to update generated files.
- `make genproto` — regenerate all proto-generated files (dev, benchmark, tests, examples).
- `make release-pr VERSION=vX.Y.Z` — create a release branch, commit changes and open a PR (requires a clean working tree).
- `make release-publish VERSION=vX.Y.Z` — create and push an annotated tag; then use `gh release create` to publish the release notes.

## Quick flow

```shell
# Prepare everything and get a suggested version from gorelease
make prepare-release

# If the suggested version looks good, edit version constants as instructed
# then update generated files and tidy modules:
make genproto
go mod tidy
(cd examples && go mod tidy)

# (Optional) Run tests
make test
make testrace

# Create the release PR (when ready)
make release-pr VERSION=v0.9.0

# After merge, tag & push and publish the release notes with gh
make release-publish VERSION=v0.9.0
gh release create v0.9.0 --prerelease --title "Gorums v0.9.0" --notes-file release-notes.md

# To check that the new version is available (after a bit of time):
go list -m github.com/relab/gorums@v0.9.0
```

## Version file edits

After `make prepare-release` prints the suggested version, update the version constants in the following files before creating the PR:

- `internal/version/version.go`
- `version.go` (keep `MinVersion` unchanged unless you intentionally want to relax the minimum)

After editing those files, regenerate generated files and tidy modules as shown above, then create the PR with `make release-pr`.
