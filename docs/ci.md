# Continuous integration

One GitHub Actions workflow, `.github/workflows/release.yml`, tests the app on Windows, macOS and Linux, builds the three release files and publishes them as a GitHub Release.

Every build goes through the `Makefile`, so CI and local builds agree. `VERSION`, `COMMIT` and `DATE` are injected into `github.com/mihaiflorentin/wc3-team-follow/cmd/cli` with `-ldflags -X`; the app reports them with `--version` and in its diagnostic report. The Go version comes from `go.mod`.

## Triggers

| Push | Result |
| --- | --- |
| A commit to `master` | Replaces the rolling **`edge`** pre-release with fresh builds, versioned `0.0.0-edge.<sha>`. Its notes are `CHANGELOG.md`'s `[Unreleased]` section. |
| A Release Tag (`v1.2.3`, `1.2.3`, `v1.2.3-rc.1`, …) | Publishes a release named after the tag, with the tag's `CHANGELOG.md` section as its notes (see [Release notes](#release-notes)). Pre-release tags are marked as pre-releases. |
| Anything else (other branches, pull requests, non-semver tags) | Nothing. |

It can also be started by hand (`workflow_dispatch`); from `master` that refreshes `edge`.

## Jobs

| Job | Runner | What it does |
| --- | --- | --- |
| `classify` | ubuntu | Decides the release tag and version. Tags go through `scripts/release-tag.sh`; tags it rejects stop every later job. Then writes the release notes with `scripts/releasenotes` and uploads them as the `notes` artifact; a tag without a `CHANGELOG.md` section fails here, before anything is built. |
| `test` | ubuntu-22.04, macos, windows | `go vet ./...` and `go test ./...` on each OS. Linux installs `libgtk-3-dev libwebkit2gtk-4.1-dev` first. |
| `build` | see below | `make package-<target>` and uploads the file. |
| `release` | ubuntu | Downloads the three files and the notes, writes `checksums.txt` (SHA-256) and runs `gh release create --notes-file`. For `edge` it first deletes the previous `edge` release and tag. |

Build matrix:

| File | Runner | Why there |
| --- | --- | --- |
| `wc3-team-follow-windows-amd64.zip` | ubuntu-latest | Windows needs no cgo, so it cross-compiles (`-H windowsgui`). `make winres` first compiles the icon, a DPI-aware manifest and the version (from the tag) into `rsrc_windows_amd64.syso` with `go-winres`, which `go build` links into the `.exe`. |
| `wc3-team-follow-linux-amd64.tar.gz` | ubuntu-22.04 | cgo and WebKitGTK 4.1; the oldest supported Ubuntu keeps the glibc requirement low. The tarball holds `wc3-team-follow`, its icon and a `.desktop` entry. |
| `wc3-team-follow-macos-arm64.zip` | macos-latest (Apple silicon) | cgo and Cocoa; `make package-darwin-arm64` builds the `.app` with `AppIcon.icns`, ad-hoc signs it with `packaging/darwin/entitlements.plist` and zips it with `ditto`. |

Jobs run in order (`needs:`), so a failing test on any OS publishes nothing.

## Cutting a release

1. Make sure `master` is green.
2. In `CHANGELOG.md`, rename `## [Unreleased]` to `## [X.Y.Z] - YYYY-MM-DD`, add a new empty `## [Unreleased]` above it and update the compare links at the bottom (see "Changelog" in `AGENTS.md`). Check the notes: `go run ./scripts/releasenotes -tag vX.Y.Z`. Commit and push.
3. Tag the commit and push the tag:

   ```bash
   git tag v0.5.0
   git push origin v0.5.0
   ```

If any job fails, no release is created. Fix the problem, delete the tag (`git push --delete origin v0.5.0 && git tag -d v0.5.0`) and tag again.

## Release notes

`scripts/releasenotes` (a small Go command, tested with `go test ./...`) prints a release's notes from `CHANGELOG.md`, the same file the app builds in for What's new:

```bash
go run ./scripts/releasenotes -tag v0.7.0              # a release
go run ./scripts/releasenotes -tag edge -head abc1234  # the edge pre-release
```

- A tag's notes are its section's lines below the `## [X.Y.Z] - date` heading (the tag without the `v`), then `**Full Changelog**: https://github.com/<repo>/compare/<previous>...<tag>`. The previous tag is the next version below it in `CHANGELOG.md` with the tag's `v` prefix, so a version that was tagged but never released (v0.5.1) is skipped. The oldest version has no compare link.
- `edge` gets a line saying it is an untested build, the `[Unreleased]` section (or "No changes since X.Y.Z yet.") and a compare link from the newest version to the commit.
- A tag with no section, or a section without entries, exits with an error naming the missing `## [X.Y.Z]` heading, and the workflow stops with a `CHANGELOG.md` annotation. Fix the changelog, delete the tag and tag again.

The output for v0.5.0 to v0.7.0 matches the notes those releases were given by hand.

## Release tags

`scripts/release-tag.sh <tag>` accepts a semantic version with an optional leading `v` and prints `release`, `prerelease` and `environment` as `key=value` lines. This workflow uses `release` and `prerelease`.

| Tag | Release | Pre-release |
| --- | --- | --- |
| `1.2.3`, `v1.2.3` | yes | no |
| `1.2.3-beta.2`, `1.2.3-rc.1`, any pre-release part | yes | yes |
| `feature-x`, `1.2`, `edge` | no | no |

## Permissions

The workflow defaults to `contents: read`. Only `release` gets `contents: write`, to create releases and the `edge` tag, using the built-in `GITHUB_TOKEN`. If the repository restricts workflow permissions, allow "Read and write permissions" under Settings → Actions → General.

## Signing

Nothing is code-signed with a paid certificate. Windows shows SmartScreen once; macOS needs right-click → Open (the app is ad-hoc signed and not notarized). `docs/install.md` tells players what to do. Adding real signing means adding secrets and a signing step to the `build` job.
