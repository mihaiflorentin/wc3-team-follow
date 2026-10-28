package main

import (
	"os"
	"strings"
	"testing"
)

const repo = "mihaiflorentin/wc3-team-follow"

const sample = `# Changelog

Prose about the format.

## [Unreleased]

### Added

- **Blacklist.** Keeps players out.

## [0.7.0] - 2026-10-28

### Added

- **Recent players.** Lists everyone.
- The Friends tab shows how many friends are online.

## [0.5.2] - 2026-10-27

### Fixed

- Auto-host opens lobbies correctly.

## [0.5.0] - 2026-10-27

### Added

- Rebuilt in Go.

[Unreleased]: https://github.com/mihaiflorentin/wc3-team-follow/compare/v0.7.0...HEAD
[0.7.0]: https://github.com/mihaiflorentin/wc3-team-follow/compare/v0.5.2...v0.7.0
`

func TestNotesForATagAreItsSectionAndTheCompareLink(t *testing.T) {
	got, err := Notes(sample, "v0.7.0", repo, "")
	want := "### Added\n\n- **Recent players.** Lists everyone.\n- The Friends tab shows how many friends are online.\n\n" +
		"**Full Changelog**: https://github.com/mihaiflorentin/wc3-team-follow/compare/v0.5.2...v0.7.0\n"
	if err != nil || got != want {
		t.Fatalf("Notes =\n%q, %v\nwant\n%q", got, err, want)
	}
}

// The previous release is the version below in the changelog, so a tag
// that was never released (v0.5.1) is skipped.
func TestNotesCompareWithThePreviousVersionInTheChangelog(t *testing.T) {
	got, err := Notes(sample, "v0.5.2", repo, "")
	if err != nil || !strings.HasSuffix(got, "compare/v0.5.0...v0.5.2\n") {
		t.Fatalf("Notes = %q, %v", got, err)
	}
	got, _ = Notes(sample, "0.5.2", repo, "")
	if !strings.HasSuffix(got, "compare/0.5.0...0.5.2\n") {
		t.Fatalf("without v = %q", got)
	}
}

func TestTheFirstReleaseHasNoCompareLink(t *testing.T) {
	got, err := Notes(sample, "v0.5.0", repo, "")
	if err != nil || got != "### Added\n\n- Rebuilt in Go.\n" {
		t.Fatalf("Notes = %q, %v", got, err)
	}
}

func TestATagWithoutASectionFails(t *testing.T) {
	for _, tag := range []string{"v0.8.0", "v0.7.0-rc.1", "0.7"} {
		_, err := Notes(sample, tag, repo, "")
		if err == nil || !strings.Contains(err.Error(), "CHANGELOG.md has no") {
			t.Errorf("%s: err = %v", tag, err)
		}
	}
	_, err := Notes("## [0.8.0] - 2026-11-01\n\n## [0.7.0]\n- Old.\n", "v0.8.0", repo, "")
	if err == nil || !strings.Contains(err.Error(), "is empty") {
		t.Fatalf("empty section: err = %v", err)
	}
}

func TestEdgeUsesUnreleased(t *testing.T) {
	got, err := Notes(sample, "edge", repo, "abc1234")
	want := edgeIntro + "\n\n### Added\n\n- **Blacklist.** Keeps players out.\n\n" +
		"**Full Changelog**: https://github.com/mihaiflorentin/wc3-team-follow/compare/v0.7.0...abc1234\n"
	if err != nil || got != want {
		t.Fatalf("Notes =\n%q, %v\nwant\n%q", got, err, want)
	}
	got, err = Notes(strings.Replace(sample, "- **Blacklist.** Keeps players out.", "", 1), "edge", repo, "abc1234")
	if err != nil || !strings.Contains(got, "No changes since 0.7.0 yet.") {
		t.Fatalf("empty Unreleased = %q, %v", got, err)
	}
}

// The repository's own changelog has a section for every release.
func TestTheRepositoryChangelogHasNotesForV070(t *testing.T) {
	text, err := os.ReadFile("../../CHANGELOG.md")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Notes(string(text), "v0.7.0", repo, "")
	if err != nil || !strings.HasPrefix(got, "### Added\n\n- **Recent players.**") ||
		!strings.HasSuffix(got, "compare/v0.6.1...v0.7.0\n") {
		t.Fatalf("Notes = %q, %v", got, err)
	}
}
