package update

import (
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// Limits on what ParseChangelog keeps, since the text comes from the
// network.
const (
	maxSections = 50
	maxGroups   = 10
	maxEntries  = 100
	maxEntryLen = 1000
)

// ParseChangelog reads CHANGELOG.md in the Keep a Changelog format: a
// "## [X.Y.Z] - date" heading per released version, "### Added"-style
// groups and "- " entries. Unreleased and pre-release sections, link
// definitions and other prose are left out. Entries become plain-text
// spans: **bold** and `code` are kept as flags, links keep only their text,
// and anything else (HTML included) stays literal text.
func ParseChangelog(text string) []dto.ChangelogSection {
	text = strings.ToValidUTF8(text, "")
	sections := []dto.ChangelogSection{}
	var section *dto.ChangelogSection
	var raw *string // the entry being read, for continuation lines
	var entries []*string
	var groupOf []int

	flush := func() {
		if section == nil {
			return
		}
		for i, e := range entries {
			g := &section.Groups[groupOf[i]]
			g.Entries = append(g.Entries, dto.ChangelogEntry{Spans: inline(clip(*e))})
		}
		section.Groups = slices.DeleteFunc(section.Groups, func(g dto.ChangelogGroup) bool { return len(g.Entries) == 0 })
		if len(section.Groups) > 0 && len(sections) < maxSections {
			sections = append(sections, *section)
		}
		section, raw, entries, groupOf = nil, nil, nil, nil
	}

	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimRight(line, "\r \t")
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "## "):
			flush()
			if version, date, ok := sectionHeading(line[3:]); ok {
				section = &dto.ChangelogSection{Version: version, Date: date}
			}
		case section == nil:
		case strings.HasPrefix(line, "### "):
			raw = nil
			if len(section.Groups) < maxGroups {
				section.Groups = append(section.Groups, dto.ChangelogGroup{Name: clip(strings.TrimSpace(line[4:]))})
			}
		case strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* "):
			if len(section.Groups) == 0 {
				section.Groups = append(section.Groups, dto.ChangelogGroup{})
			}
			g := len(section.Groups) - 1
			if count(groupOf, g) >= maxEntries {
				raw = nil
				continue
			}
			text := strings.TrimSpace(line[2:])
			raw = &text
			entries, groupOf = append(entries, raw), append(groupOf, g)
		case raw != nil && trimmed != "" && line != trimmed:
			if len(*raw) < maxEntryLen {
				*raw += " " + trimmed
			}
		default:
			raw = nil
		}
	}
	flush()
	return sections
}

func count(groups []int, g int) int {
	n := 0
	for _, x := range groups {
		if x == g {
			n++
		}
	}
	return n
}

// sectionHeading reads "[X.Y.Z] - date", "X.Y.Z - date" or "[X.Y.Z]".
func sectionHeading(h string) (version, date string, ok bool) {
	name, rest, _ := strings.Cut(strings.TrimSpace(h), " ")
	name = strings.TrimSuffix(strings.TrimPrefix(name, "["), "]")
	v, ok := ParseVersion(name)
	if !ok {
		return "", "", false
	}
	date = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rest), "-"))
	if len(date) > 20 {
		date = ""
	}
	return v.String(), date, true
}

// clip shortens s to maxEntryLen bytes, on a character boundary.
func clip(s string) string {
	if len(s) <= maxEntryLen {
		return s
	}
	s = s[:maxEntryLen]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

// inline splits one entry into plain, bold and code spans. Markers without
// a closing partner stay as text.
func inline(s string) []dto.TextSpan {
	var spans []dto.TextSpan
	var text strings.Builder
	push := func(span dto.TextSpan) {
		if span.Text == "" {
			return
		}
		if n := len(spans); n > 0 && !span.Bold && !span.Code && !spans[n-1].Bold && !spans[n-1].Code {
			spans[n-1].Text += span.Text
			return
		}
		spans = append(spans, span)
	}
	flushText := func() {
		push(dto.TextSpan{Text: text.String()})
		text.Reset()
	}
	for i := 0; i < len(s); {
		switch {
		case strings.HasPrefix(s[i:], "**"):
			if end := strings.Index(s[i+2:], "**"); end > 0 {
				flushText()
				push(dto.TextSpan{Text: linkText(s[i+2 : i+2+end]), Bold: true})
				i += end + 4
				continue
			}
		case s[i] == '`':
			if end := strings.IndexByte(s[i+1:], '`'); end > 0 {
				flushText()
				push(dto.TextSpan{Text: s[i+1 : i+1+end], Code: true})
				i += end + 2
				continue
			}
		case s[i] == '[':
			if label, n, ok := link(s[i:]); ok {
				text.WriteString(label)
				i += n
				continue
			}
		}
		text.WriteByte(s[i])
		i++
	}
	flushText()
	if spans == nil {
		spans = []dto.TextSpan{}
	}
	return spans
}

// link reads "[label](target)" at the start of s and returns the label and
// the length read.
func link(s string) (label string, n int, ok bool) {
	closeLabel := strings.Index(s, "](")
	if closeLabel < 1 || strings.ContainsAny(s[1:closeLabel], "[]") {
		return "", 0, false
	}
	closeTarget := strings.IndexByte(s[closeLabel:], ')')
	if closeTarget < 0 {
		return "", 0, false
	}
	return s[1:closeLabel], closeLabel + closeTarget + 1, true
}

// linkText keeps only the labels of links inside a bold span.
func linkText(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '[' {
			if label, n, ok := link(s[i:]); ok {
				b.WriteString(label)
				i += n
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// ChangesBetween returns the sections newer than current, up to and
// including offered, newest first. It never returns nil.
func ChangesBetween(sections []dto.ChangelogSection, current, offered Version) []dto.ChangelogSection {
	out := []dto.ChangelogSection{}
	for _, s := range sections {
		v, ok := ParseVersion(s.Version)
		if ok && v.Newer(current) && !v.Newer(offered) {
			out = append(out, s)
		}
	}
	slices.SortStableFunc(out, func(a, b dto.ChangelogSection) int {
		va, _ := ParseVersion(a.Version)
		vb, _ := ParseVersion(b.Version)
		switch {
		case va.Newer(vb):
			return -1
		case vb.Newer(va):
			return 1
		}
		return 0
	})
	return out
}
