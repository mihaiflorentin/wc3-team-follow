// Command releasenotes prints a GitHub Release's notes from CHANGELOG.md:
// the tag's section, then the "Full Changelog" link that compares it with
// the previous version in the changelog. The edge pre-release gets the
// [Unreleased] section. A tag without a section, or with an empty one,
// fails. The release workflow runs it; see docs/ci.md.
//
//	go run ./scripts/releasenotes -tag v0.8.0 > notes.md
//	go run ./scripts/releasenotes -tag edge -head "$GITHUB_SHA" > notes.md
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// edgeIntro opens the edge pre-release's notes.
const edgeIntro = "Built automatically from the latest commit on master. Not a tested release; see docs/install.md for setup."

func main() {
	tag := flag.String("tag", "", "release tag, such as v0.8.0, or edge")
	file := flag.String("changelog", "CHANGELOG.md", "changelog file")
	repo := flag.String("repo", "mihaiflorentin/wc3-team-follow", "GitHub repository, owner/name")
	head := flag.String("head", "", "commit the edge build was made from")
	flag.Parse()
	text, err := os.ReadFile(*file)
	if err == nil {
		var notes string
		if notes, err = Notes(string(text), *tag, *repo, *head); err == nil {
			_, err = fmt.Print(notes)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "releasenotes:", err)
		os.Exit(1)
	}
}

// section is one "## " heading of the changelog and the lines below it.
type section struct {
	name string
	body string
}

var (
	heading = regexp.MustCompile(`^## \[?([^\]\s]+)\]?`)
	// linkDef is a Markdown link definition, like the compare links at the
	// end of the changelog.
	linkDef = regexp.MustCompile(`^\[[^\]]+\]:\s`)
	version = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
)

// sections reads the changelog's "## " sections in file order.
func sections(text string) []section {
	var out []section
	var lines []string
	flush := func() {
		// Group headings without entries leave the section empty.
		empty := true
		for _, l := range lines {
			if strings.TrimSpace(l) != "" && !strings.HasPrefix(l, "### ") {
				empty = false
			}
		}
		if len(out) > 0 && !empty {
			out[len(out)-1].body = strings.TrimSpace(strings.Join(lines, "\n"))
		}
		lines = nil
	}
	for line := range strings.SplitSeq(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line = strings.TrimRight(line, " \t")
		if m := heading.FindStringSubmatch(line); m != nil {
			flush()
			out = append(out, section{name: m[1]})
			continue
		}
		if linkDef.MatchString(line) {
			continue
		}
		lines = append(lines, line)
	}
	flush()
	return out
}

// Notes returns the release notes for tag: its CHANGELOG.md section
// and the "Full Changelog" compare link with the previous version below
// it, which uses the tag's v prefix. The first release has no link. For
// "edge", the [Unreleased] section and a link from the newest version to
// head.
func Notes(changelog, tag, repo, head string) (string, error) {
	all := sections(changelog)
	compare := func(from, to string) string {
		return fmt.Sprintf("**Full Changelog**: https://github.com/%s/compare/%s...%s\n", repo, from, to)
	}
	if tag == "edge" {
		body, latest := "", ""
		for _, s := range all {
			switch {
			case strings.EqualFold(s.name, "Unreleased"):
				body = s.body
			case latest == "" && version.MatchString(s.name):
				latest = s.name
			}
		}
		if body == "" {
			body = "No changes since " + latest + " yet."
		}
		notes := edgeIntro + "\n\n" + body + "\n"
		if latest != "" && head != "" {
			notes += "\n" + compare("v"+latest, head)
		}
		return notes, nil
	}

	want := strings.TrimPrefix(tag, "v")
	prefix := strings.TrimSuffix(tag, want)
	for i, s := range all {
		if s.name != want {
			continue
		}
		if s.body == "" {
			return "", fmt.Errorf("the CHANGELOG.md section for %s is empty. Write its entries, then tag again", tag)
		}
		notes := s.body + "\n"
		for _, older := range all[i+1:] {
			if version.MatchString(older.name) {
				notes += "\n" + compare(prefix+older.name, tag)
				break
			}
		}
		return notes, nil
	}
	return "", errors.New(`CHANGELOG.md has no "## [` + want + `]" section for tag ` + tag +
		`. Rename [Unreleased] to [` + want + `] with today's date (see "Changelog" in AGENTS.md), then delete the tag and tag again`)
}
