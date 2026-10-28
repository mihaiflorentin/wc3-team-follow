# 9. Self-update: checking GitHub Releases and asking the player

Status: proposed (2026-10-28). Issues 21 (spec) and 22 (the check and the prompt). This draft covers the check; the in-app download, install and restart (issue 23) add their part here.

## Context

New versions come out often, but players only find out by chance, and many run old versions with bugs that are already fixed. Releases are published on GitHub by `.github/workflows/release.yml`: a Release Tag (`X.Y.Z`, optional `v`) makes a stable release with `wc3-team-follow-windows-amd64.zip`, `wc3-team-follow-macos-arm64.zip`, `wc3-team-follow-linux-amd64.tar.gz` and `checksums.txt`; every push to `master` replaces the rolling `edge` pre-release. `CHANGELOG.md` (Keep a Changelog) holds what changed in each version, written for players.

Until now Team Follow talked only to the game, over loopback. Checking for updates is its first request to the internet, so what it sends and where must be narrow.

## Decision

- **Port.** `contract.Updater` has `Latest(ctx)`, which returns the newest stable release (`dto.Release`: tag, notes, the CHANGELOG.md text at that tag, the release page, this system's file and `checksums.txt`). Issue 23 adds `Download`, `Install` and `Restart` to the same port. A rate-limit error implements `contract.RetryLater` (`RetryAt`), so the domain waits without parsing messages.
- **Adapter** (`infrastructure/updater`). It reads `GET /repos/{repo}/releases/latest` from `api.github.com`, which skips drafts and pre-releases such as `edge`; it checks `draft` and `prerelease` again. It then reads `CHANGELOG.md` at the release's tag through the contents API (`?ref=<tag>`, `Accept: application/vnd.github.raw+json`); if that fails, the release still counts and its notes stand in. The asset for this system is picked by name (`AssetName`); a release without one, or without `checksums.txt`, leaves those fields empty. Bodies are capped (1 MB release, 512 KB changelog).
- **Network rule.** Only GitHub's own hosts, over HTTPS: the API base must be `https` (the container refuses anything else), the release page and file links must be `https` on `github.com`, `objects.githubusercontent.com` or `release-assets.githubusercontent.com` (others are dropped), and the HTTP client follows redirects only to those hosts. Requests carry GitHub's `Accept`, `X-GitHub-Api-Version` and a `wc3-team-follow` User-Agent, and nothing else: no token, cookie, BattleTag, game data or identifier. Each request times out after `[updates] timeout` (20 s).
- **Decisions in the domain** (`domain/update`, pure, driven by the companion with its clock):
  - The running version must be a clean `vX.Y.Z`. A development build (`dev`, a `git describe` version like `v0.7.0-3-gabc1234-dirty`) or an `edge` build (`0.0.0-edge.<sha>`) never checks, and the window says updates are off.
  - A release is offered only when its tag is a clean version newer than the running one (numeric comparison of major, minor, patch) and it has this system's file and `checksums.txt`, which the in-app update needs.
  - Automatic checks run at start-up and every `[updates] check_interval` (6 h) while "Check automatically" is on (the default). A failed check is quiet in the window, is logged, and is retried after `retry_interval` (30 min), or once GitHub's rate limit lifts if that is later. A failure keeps a release already found.
  - "Check for updates" always works on a release build, also with automatic checks off, and answers in the Updates section: up to date, couldn't reach GitHub, rate-limited, or no file for this system. Only one check runs at a time.
  - **Later** hides the banner until a newer release is found or the player checks again; it isn't saved. **Skip this version** saves `skippedVersion`; that version never shows the banner again, a newer one does. The Updates section still shows a skipped or postponed release with an Update button.
  - The banner warns when updating would interrupt play: in a lobby or a match, or with Auto-join or Auto-host on, it says that updating restarts Team Follow and turns Follow off, as every start does.
  - What's new: the changelog sections newer than the running version, up to and including the offered one, newest first (`ChangesBetween`). Without a usable changelog, the release notes are shown instead, as plain text.
- **No Markdown or HTML from the network reaches the page.** `ParseChangelog` turns entries into plain-text spans with only a bold or code flag (link targets dropped, unclosed marks and HTML kept as literal text, sizes capped), and `PlainNotes` flattens the release notes. The page builds nodes with `textContent` only.
- **Saved settings.** `checkUpdates` (stored only when off, so older files load with checks on) and `skippedVersion`, in `friends.json` with the other preferences.
- **Window.** A banner above the status card ("Team Follow X.Y.Z is available", the running version and release date, Update, Later, Skip this version, the warning, and a collapsible What's new with one collapsible section per version) and an Updates section beside Activity (version, status, Check for updates, Check automatically, the releases page). In this ticket Update opens the release page in the browser; the bridge opens only `https://github.com/` pages taken from the companion's state, never an address from the page.
- **Logging.** `update.check` (manual, latest tag, offered version, whether a file and changelog were found), `update.check_failed`, `update.changelog`, `update.later`, `update.skip`. Nothing personal is involved.

## Consequences

- Team Follow now makes outbound HTTPS requests to GitHub, a few per day at most, unless the player turns automatic checks off. Unauthenticated API calls share GitHub's 60-per-hour limit per IP address, which the retry rule respects.
- Nothing is downloaded or changed by the check. The in-app update (issue 23) replaces "Update opens the release page" with download, SHA-256 check against `checksums.txt`, install and restart, adding the downloading, verifying, installing, restarting and failed phases after `available`.
- Development and `edge` builds never see an update; testers on `edge` update by hand.
- Releases must keep the asset names and `checksums.txt`, and the release tag's version needs a matching `CHANGELOG.md` section, or the prompt falls back to the release notes.
