# Release v0.9.0-poppler24-02-0-202605.1

Korean localization: [release_0.9.0-poppler24-02-0-202605.1.ko.md](release_0.9.0-poppler24-02-0-202605.1.ko.md).

## Scope

This release focuses on rendering-parity infrastructure, no-CGo release validation, and automated packaging through GitHub Actions.

## Highlights

- Added Poppler/Ours/XOR HTML comparison automation.
- Added long-running failed-document recheck automation.
- Added nightly render-diff gates.
- Added no-CGo core coverage and release validation gates.
- Improved image decoder performance in render benchmarks.
- Reached zero missing godoc comments for public symbols in the project scan.
- Added tag-driven GitHub Release artifact generation.

## Validation

Run the release gate locally:

```bash
make release-ci
```

Build local release artifacts:

```bash
make release-package RELEASE_VERSION=v0.9.0-poppler24-02-0-202605.1
```

## Tag Release

```bash
git tag -a v0.9.0-poppler24-02-0-202605.1 -m "release: v0.9.0-poppler24-02-0-202605.1"
git push origin v0.9.0-poppler24-02-0-202605.1
```

The pushed tag triggers the GitHub Actions release workflow, which validates the tag, builds release binaries, packages artifacts, writes checksums, and creates the GitHub Release.

## Automated Release

Use the `Bump Release Tag` GitHub Actions workflow with `dry_run=false` to compute and push the next release-train tag automatically.

Dry-run mode validates the release gate and creates only a local tag inside the workflow runner:

```bash
gh workflow run bump-release.yml --repo dh-kam/pdf-go --ref main -f bump=patch -f dry_run=true
```

## Go Module Check

After release, verify proxy visibility:

```bash
GONOSUMDB=github.com/dh-kam/pdf-go go list -m github.com/dh-kam/pdf-go/pkg/pdf@v0.9.0-poppler24-02-0-202605.1
GOPROXY=https://proxy.golang.org go list -m github.com/dh-kam/pdf-go/pkg/pdf@v0.9.0-poppler24-02-0-202605.1
```

## Post-Release Checklist

- [ ] Confirm GitHub Release notes.
- [ ] Confirm release assets for linux, darwin, and windows.
- [ ] Confirm checksums.
- [ ] Confirm Go module proxy availability.
