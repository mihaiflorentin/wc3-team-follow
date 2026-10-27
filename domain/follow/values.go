package follow

import (
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// MaxTags is how many friend BattleTags can be added by hand.
const MaxTags = 12

var (
	tagPattern       = regexp.MustCompile(`^[^\s#]{2,32}#\d{3,12}$`)
	accountIDPattern = regexp.MustCompile(`^\d{1,20}$`)
)

// NormalizeTags trims and validates BattleTags and drops case-insensitive
// duplicates, keeping the first spelling.
func NormalizeTags(input []string) ([]string, error) {
	if len(input) > MaxTags {
		return nil, errors.New("Add up to 12 friend BattleTags.")
	}
	tags := make([]string, 0, len(input))
	seen := map[string]bool{}
	for _, raw := range input {
		tag := strings.TrimSpace(raw)
		if !tagPattern.MatchString(tag) {
			return nil, errors.New("Enter a full BattleTag, such as Friend#1234.")
		}
		if seen[tagKey(tag)] {
			continue
		}
		seen[tagKey(tag)] = true
		tags = append(tags, tag)
	}
	return tags, nil
}

// The game's payloads are loosely typed JSON. These helpers read them the
// way the game's own menu code does: a value of the wrong type counts as
// absent.

func object(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func list(v any) ([]any, bool) {
	l, ok := v.([]any)
	return l, ok
}

func field(m map[string]any, path ...string) any {
	var v any = m
	for _, key := range path {
		o := object(v)
		if o == nil {
			return nil
		}
		v = o[key]
	}
	return v
}

func isTrue(v any) bool {
	b, ok := v.(bool)
	return ok && b
}

// attribute returns a rich-presence attribute's string value, or "".
func attribute(friend map[string]any, key string) string {
	attrs, _ := list(friend["localRichPresenceAttributes"])
	for _, raw := range attrs {
		a := object(raw)
		if a["key"] == key {
			s, _ := a["value"].(string)
			return s
		}
	}
	return ""
}

func numberIs(v any, want int64) bool {
	switch n := v.(type) {
	case json.Number:
		got, err := n.Int64()
		return err == nil && got == want
	case float64:
		return n == float64(want)
	case int:
		return int64(n) == want
	case int64:
		return n == want
	}
	return false
}

// validAccountID accepts a positive integer that fits a JavaScript safe
// integer, or a string of up to 20 digits that is not all zeros.
func validAccountID(v any) bool {
	const maxSafe = 1<<53 - 1
	switch id := v.(type) {
	case json.Number:
		n, err := strconv.ParseInt(id.String(), 10, 64)
		return err == nil && n > 0 && n <= maxSafe
	case float64:
		return id > 0 && id <= maxSafe && id == math.Trunc(id)
	case int:
		return id > 0 && int64(id) <= maxSafe
	case int64:
		return id > 0 && id <= maxSafe
	case string:
		return accountIDPattern.MatchString(id) && strings.Trim(id, "0") != ""
	}
	return false
}

func itoa(n int) string { return strconv.Itoa(n) }
