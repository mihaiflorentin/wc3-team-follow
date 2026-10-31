package mapupdate

import (
	"cmp"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// Match returns the index of the first tracked map whose pattern a map
// file's name matches, and the version in that name; -1 and "" when none
// does. path may be a full path with either separator; only its last part
// is matched. Auto-host uses it to tell whether the chosen map is tracked
// (issue 32).
func Match(tracked []dto.TrackedMap, path string) (int, string) {
	name := path[strings.LastIndexAny(path, `/\`)+1:]
	if name == "" {
		return -1, ""
	}
	for i, m := range tracked {
		re, err := regexp.Compile(m.Pattern)
		if err != nil {
			continue // the container refuses these; never match one
		}
		group := re.SubexpIndex("version")
		if group < 0 {
			continue
		}
		if sub := re.FindStringSubmatch(name); sub != nil && sub[group] != "" {
			return i, sub[group]
		}
	}
	return -1, ""
}

// Newest returns the newest of a tracked map's installed files: the highest
// version by CompareVersions, and of equal versions the most recently
// changed. It is false when there are none. The time a file changed alone
// doesn't decide: the game also downloads old versions, when the player
// joins a lobby hosted on one.
func Newest(installed []dto.InstalledMap) (dto.InstalledMap, bool) {
	if len(installed) == 0 {
		return dto.InstalledMap{}, false
	}
	best := installed[0]
	for _, m := range installed[1:] {
		if c := CompareVersions(m.Version, best.Version); c > 0 || c == 0 && m.Modified.After(best.Modified) {
			best = m
		}
	}
	return best, true
}

// CompareVersions orders two map versions such as 11.4c-hf9 and 11.4c-hf10:
// piece by piece, runs of digits as numbers and the rest as text ignoring
// case, a version that is a prefix of the other coming first (11.4c before
// 11.4c-hf1). It returns -1, 0 or 1.
func CompareVersions(a, b string) int {
	pa, pb := pieces(strings.ToLower(a)), pieces(strings.ToLower(b))
	for i := range min(len(pa), len(pb)) {
		x, y := pa[i], pb[i]
		nx, errX := strconv.ParseUint(x, 10, 64)
		ny, errY := strconv.ParseUint(y, 10, 64)
		var c int
		if errX == nil && errY == nil {
			c = cmp.Compare(nx, ny)
		} else {
			c = strings.Compare(x, y)
		}
		if c != 0 {
			return c
		}
	}
	return cmp.Compare(len(pa), len(pb))
}

// pieces splits s into runs of digits and runs of everything else.
func pieces(s string) []string {
	var out []string
	start := 0
	for i, r := range s {
		if i > start && unicode.IsDigit(r) != unicode.IsDigit(rune(s[i-1])) {
			out = append(out, s[start:i])
			start = i
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
