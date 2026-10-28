package update_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mihaiflorentin/wc3-team-follow/domain/update"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

const sampleChangelog = `# Changelog

What changed in each version, newest first.

## [Unreleased]

### Added

- Something not released yet.

## [0.8.0] - 2026-11-02

### Added

- **Update check.** Team Follow tells you when a new version is out.
- Use ` + "`Check for updates`" + ` any time.

### Fixed

- Auto-join finds lobbies
  more reliably.

## [0.7.1] - 2026-10-30

### Fixed

- A fix for Player#1234's lobby.

## [0.7.0] - 2026-10-28

### Added

- **Recent players.**

[Unreleased]: https://github.com/mihaiflorentin/wc3-team-follow/compare/v0.8.0...HEAD
[0.8.0]: https://github.com/mihaiflorentin/wc3-team-follow/compare/v0.7.1...v0.8.0
`

func plain(text string) dto.TextSpan { return dto.TextSpan{Text: text} }

func entry(spans ...dto.TextSpan) dto.ChangelogEntry { return dto.ChangelogEntry{Spans: spans} }

func TestParseChangelogReadsReleasedSectionsAndGroups(t *testing.T) {
	sections := update.ParseChangelog(sampleChangelog)
	if len(sections) != 3 {
		t.Fatalf("sections = %+v", sections)
	}
	first := sections[0]
	if first.Version != "0.8.0" || first.Date != "2026-11-02" || len(first.Groups) != 2 {
		t.Fatalf("first = %+v", first)
	}
	added := first.Groups[0]
	if added.Name != "Added" || len(added.Entries) != 2 {
		t.Fatalf("added = %+v", added)
	}
	want := entry(dto.TextSpan{Text: "Update check.", Bold: true}, plain(" Team Follow tells you when a new version is out."))
	if !reflect.DeepEqual(added.Entries[0], want) {
		t.Fatalf("bold entry = %+v", added.Entries[0])
	}
	want = entry(plain("Use "), dto.TextSpan{Text: "Check for updates", Code: true}, plain(" any time."))
	if !reflect.DeepEqual(added.Entries[1], want) {
		t.Fatalf("code entry = %+v", added.Entries[1])
	}
	fixed := first.Groups[1]
	if fixed.Name != "Fixed" || !reflect.DeepEqual(fixed.Entries, []dto.ChangelogEntry{entry(plain("Auto-join finds lobbies more reliably."))}) {
		t.Fatalf("continued entry = %+v", fixed)
	}
}

func TestChangesBetweenListsNewerSectionsUpToTheOfferedVersionNewestFirst(t *testing.T) {
	sections := update.ParseChangelog(sampleChangelog)
	v := func(s string) update.Version { out, _ := update.ParseVersion(s); return out }
	versions := func(got []dto.ChangelogSection) []string {
		out := []string{}
		for _, s := range got {
			out = append(out, s.Version)
		}
		return out
	}
	if got := versions(update.ChangesBetween(sections, v("0.7.0"), v("0.8.0"))); !reflect.DeepEqual(got, []string{"0.8.0", "0.7.1"}) {
		t.Fatalf("0.7.0 to 0.8.0 = %v", got)
	}
	if got := versions(update.ChangesBetween(sections, v("0.7.1"), v("0.8.0"))); !reflect.DeepEqual(got, []string{"0.8.0"}) {
		t.Fatalf("0.7.1 to 0.8.0 = %v", got)
	}
	// An offered version older than the newest section leaves the newer one out.
	if got := versions(update.ChangesBetween(sections, v("0.6.0"), v("0.7.1"))); !reflect.DeepEqual(got, []string{"0.7.1", "0.7.0"}) {
		t.Fatalf("0.6.0 to 0.7.1 = %v", got)
	}
	if got := update.ChangesBetween(sections, v("0.8.0"), v("0.8.0")); got == nil || len(got) != 0 {
		t.Fatalf("nothing newer = %#v", got)
	}
}

func TestChangesBetweenSortsSectionsWrittenOutOfOrder(t *testing.T) {
	text := "## [0.7.0] - 2026-10-28\n### Added\n- Old.\n## 0.8.0 - 2026-11-02\n### Added\n- New.\n"
	from, _ := update.ParseVersion("0.6.0")
	to, _ := update.ParseVersion("0.8.0")
	got := update.ChangesBetween(update.ParseChangelog(text), from, to)
	if len(got) != 2 || got[0].Version != "0.8.0" || got[1].Version != "0.7.0" {
		t.Fatalf("got %+v", got)
	}
}

func TestParseChangelogKeepsMarkupAsPlainText(t *testing.T) {
	text := "## [0.8.0]\n### Added\n- <img src=x onerror=alert(1)> and **unclosed bold\n- See [the guide](https://example.com/guide) for _more_.\n"
	sections := update.ParseChangelog(text)
	if len(sections) != 1 || sections[0].Date != "" {
		t.Fatalf("sections = %+v", sections)
	}
	entries := sections[0].Groups[0].Entries
	if !reflect.DeepEqual(entries[0], entry(plain("<img src=x onerror=alert(1)> and **unclosed bold"))) {
		t.Fatalf("html entry = %+v", entries[0])
	}
	if !reflect.DeepEqual(entries[1], entry(plain("See the guide for _more_."))) {
		t.Fatalf("link entry = %+v", entries[1])
	}
}

func TestParseChangelogIgnoresMalformedInput(t *testing.T) {
	for name, text := range map[string]string{
		"empty":           "",
		"no sections":     "# Changelog\n\n- A stray line.\n",
		"not a version":   "## [next] - soon\n### Added\n- Thing.\n",
		"pre-release":     "## [1.0.0-beta.1] - 2026-11-01\n### Added\n- Thing.\n",
		"binary garbage":  "\x00\xff## [\x00",
		"only unreleased": "## [Unreleased]\n### Added\n- Thing.\n",
	} {
		if got := update.ParseChangelog(text); len(got) != 0 {
			t.Errorf("%s: sections = %+v", name, got)
		}
	}
	// Entries before any group land in an unnamed group; empty sections stay out.
	got := update.ParseChangelog("## [0.8.0]\n- Loose entry.\n## [0.7.0]\n\n")
	if len(got) != 1 || got[0].Groups[0].Name != "" || len(got[0].Groups[0].Entries) != 1 {
		t.Fatalf("loose = %+v", got)
	}
}

func TestParseChangelogLimitsHugeInput(t *testing.T) {
	var b strings.Builder
	b.WriteString("## [0.8.0]\n### Added\n")
	for range 1000 {
		b.WriteString("- " + strings.Repeat("x", 5000) + "\n")
	}
	got := update.ParseChangelog(b.String())
	entries := got[0].Groups[0].Entries
	if len(entries) > 100 || len(entries[0].Spans[0].Text) > 1000 {
		t.Fatalf("kept %d entries, first %d characters", len(entries), len(entries[0].Spans[0].Text))
	}
}
