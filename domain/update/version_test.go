package update_test

import (
	"testing"

	"github.com/mihaiflorentin/wc3-team-follow/domain/update"
)

func TestParseVersionAcceptsOnlyCleanReleaseVersions(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
		ok   bool
	}{
		{"v0.7.0", "0.7.0", true},
		{"0.7.0", "0.7.0", true},
		{"v10.20.30", "10.20.30", true},
		{"dev", "", false},
		{"", "", false},
		{"edge", "", false},
		{"0.0.0-edge.abc1234", "", false},
		{"v0.7.0-3-gabc1234", "", false},
		{"v0.7.0-dirty", "", false},
		{"v0.7.0-beta.1", "", false},
		{"v0.7", "", false},
		{"v01.2.3", "", false},
		{"abc1234", "", false},
		{" v0.7.0", "", false},
	} {
		v, ok := update.ParseVersion(tc.in)
		if ok != tc.ok || (ok && v.String() != tc.want) {
			t.Errorf("ParseVersion(%q) = %v, %v; want %q, %v", tc.in, v, ok, tc.want, tc.ok)
		}
	}
}

func TestVersionsCompareNumerically(t *testing.T) {
	for _, tc := range []struct {
		a, b  string
		newer bool
	}{
		{"0.8.0", "0.7.0", true},
		{"0.10.0", "0.9.0", true},
		{"1.0.0", "0.99.99", true},
		{"0.7.1", "0.7.0", true},
		{"0.7.0", "0.7.0", false},
		{"0.6.9", "0.7.0", false},
	} {
		a, _ := update.ParseVersion(tc.a)
		b, _ := update.ParseVersion(tc.b)
		if got := a.Newer(b); got != tc.newer {
			t.Errorf("%s.Newer(%s) = %v", tc.a, tc.b, got)
		}
	}
}
