package main

import (
	"testing"

	"github.com/mihaiflorentin/wc3-team-follow/domain/update"
)

// The changelog built into the app is the repository's CHANGELOG.md, and
// What's new can read every released version in it.
func TestTheChangelogIsBuiltIn(t *testing.T) {
	sections := update.ParseChangelog(changelog)
	if len(sections) < 7 {
		t.Fatalf("built-in changelog has %d released versions", len(sections))
	}
	last := sections[len(sections)-1]
	if last.Version != "0.5.0" || last.Date != "2026-10-27" {
		t.Fatalf("oldest version = %+v", last)
	}
}
