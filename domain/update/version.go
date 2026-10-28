// Package update decides when Team Follow checks GitHub for a newer
// release, which release it offers and what the window shows about it
// (docs/adr/0009). It is pure: the companion feeds it the clock and the
// Updater's answers.
package update

import (
	"fmt"
	"strconv"
	"strings"
)

// Version is a clean release version, X.Y.Z.
type Version struct {
	Major, Minor, Patch int
}

// ParseVersion reads a clean release version, X.Y.Z with an optional
// leading v. Anything else, such as a development build ("dev", a
// git describe version like v0.7.0-3-gabc1234) or a pre-release
// (0.0.0-edge.abc1234, 1.0.0-beta.1), reports false.
func ParseVersion(s string) (Version, bool) {
	parts := strings.Split(strings.TrimPrefix(s, "v"), ".")
	if len(parts) != 3 {
		return Version{}, false
	}
	var n [3]int
	for i, p := range parts {
		if p == "" || len(p) > 6 || (len(p) > 1 && p[0] == '0') {
			return Version{}, false
		}
		for _, r := range p {
			if r < '0' || r > '9' {
				return Version{}, false
			}
		}
		n[i], _ = strconv.Atoi(p)
	}
	return Version{n[0], n[1], n[2]}, true
}

// Newer reports whether v is a later version than o.
func (v Version) Newer(o Version) bool {
	if v.Major != o.Major {
		return v.Major > o.Major
	}
	if v.Minor != o.Minor {
		return v.Minor > o.Minor
	}
	return v.Patch > o.Patch
}

// String is X.Y.Z, without a leading v.
func (v Version) String() string { return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch) }
