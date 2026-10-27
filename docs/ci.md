# Continuous integration

wc3-team-follow ships the release pipeline(s) selected when the project was generated:

- GitHub Actions: `.github/workflows/release.yml`

Every pipeline builds through the `Makefile`, so CI and local builds agree:
`make build-all` cross-compiles for linux, darwin and windows (amd64 and arm64)
into `dist/`, injecting `VERSION`, `COMMIT` and `DATE` into
`github.com/mihaiflorentin/wc3-team-follow/cmd/cli` with `-ldflags -X`. The binary reports them with
`wc3-team-follow --version`. The Go version comes from `go.mod`.

## Cutting a release

1. Make sure the default branch is green locally: `go vet ./... && go test ./...`.
2. Tag the commit with a semantic version and push the tag:

   ```bash
   git tag v1.2.3
   git push origin v1.2.3
   ```

3. The pipeline tests, builds and publishes the release. On GitHub, a
   pre-release tag (for example `v1.2.3-rc.1`) is published as a pre-release.

If any job fails, no release is created. Fix the problem, delete the tag
(`git push --delete origin v1.2.3 && git tag -d v1.2.3`) and tag again.

## Release tags

`scripts/release-tag.sh <tag>` decides what a tag does. It accepts a semantic
version with an optional leading `v` and prints `release`, `prerelease` and
`environment` as `key=value` lines (GitHub step outputs, a GitLab dotenv report
or `eval` in a shell):

| Tag | Release | Pre-release | Environment |
| --- | --- | --- | --- |
| `1.2.3`, `v1.2.3` | yes | no | prod |
| `1.2.3-beta`, `1.2.3-beta.2` | yes | yes | stage |
| `1.2.3-alpha`, `1.2.3-alpha.1` | yes | yes | dev |
| `1.2.3-rc.1` (any other pre-release) | yes | yes | none: release only |
| `feature-x`, `1.2` (anything else) | no | no | none: ignored |

Try it locally: `sh scripts/release-tag.sh v1.2.3-beta.1`.
On GitHub, every Release Tag only builds and publishes binaries: the project has
no Helm chart, so the Environment column does not apply.

## GitHub Actions

Workflow: `.github/workflows/release.yml`.

Trigger: pushing a tag matching `v[0-9]+.[0-9]+.[0-9]+*` or
`[0-9]+.[0-9]+.[0-9]+*`. Branch pushes and pull requests do not run it, and the
`classify` job stops every later job for a tag that is not a Release Tag.

Jobs, each starting only after the previous one succeeds:

| Job | What it does |
| --- | --- |
| `classify` | Runs `scripts/release-tag.sh` on the tag and exposes `release`, `prerelease` and `environment` as job outputs. |
| `test` | `go vet ./...` and `go test ./...` with the Go version from `go.mod`. Skipped (and so is everything after it) when `release` is not `true`. |
| `build` | `make build-all` with `VERSION` set to the tag, `COMMIT` to the commit SHA and `DATE` to the UTC build time, then writes `dist/checksums.txt` (SHA-256) and uploads the binaries and checksums as the `release-artifacts` artifact. |
| `release` | Downloads `release-artifacts` and runs `gh release create` with every binary and `checksums.txt` attached and release notes generated from the commits since the previous tag. Pre-release tags are marked as pre-releases. |

Permissions: the workflow defaults to `contents: read`; only the `release` job
gets `contents: write`, which it needs to create the release. It authenticates
with the built-in `GITHUB_TOKEN`. If the repository or organisation restricts
workflow permissions, allow "Read and write permissions" under Settings →
Actions → General, or keep the per-job `permissions` block as generated.

Editing tips: GitHub expressions such as `${{ github.ref_name }}` are
evaluated by Actions, not by the shell. Keep the job order (`needs:`) so a red
test never produces a release, and change
tag semantics in `scripts/release-tag.sh`, which the pipelines read.
