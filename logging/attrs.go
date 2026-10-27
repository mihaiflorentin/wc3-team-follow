package logging

import (
	"log/slog"
	"strings"
)

// boundAttr is an attribute added through Logger.With, remembered together
// with the groups that were open when it was added.
type boundAttr struct {
	groups []string
	attr   slog.Attr
}

// handlerState holds what WithAttrs and WithGroup accumulate, so the custom
// handlers keep context added through logger.With(...) and logger.WithGroup(...).
// It is immutable: every With* call returns a copy.
type handlerState struct {
	groups []string
	attrs  []boundAttr
}

func (s handlerState) withAttrs(attrs []slog.Attr) handlerState {
	if len(attrs) == 0 {
		return s
	}
	next := make([]boundAttr, len(s.attrs), len(s.attrs)+len(attrs))
	copy(next, s.attrs)
	for _, a := range attrs {
		next = append(next, boundAttr{groups: s.groups, attr: a})
	}
	return handlerState{groups: s.groups, attrs: next}
}

func (s handlerState) withGroup(name string) handlerState {
	if name == "" {
		return s
	}
	groups := make([]string, len(s.groups), len(s.groups)+1)
	copy(groups, s.groups)
	return handlerState{groups: append(groups, name), attrs: s.attrs}
}

// field is a flattened attribute whose key carries its group path ("http.method").
type field struct {
	key   string
	value slog.Value
}

// flatten returns the bound attributes followed by the record's attributes,
// with group names joined into dotted keys.
func (s handlerState) flatten(r slog.Record) []field {
	var out []field
	for _, b := range s.attrs {
		out = appendFlat(out, b.groups, b.attr)
	}
	r.Attrs(func(a slog.Attr) bool {
		out = appendFlat(out, s.groups, a)
		return true
	})
	return out
}

func appendFlat(out []field, groups []string, a slog.Attr) []field {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return out
	}
	if a.Value.Kind() == slog.KindGroup {
		inner := groups
		if a.Key != "" {
			inner = append(append([]string{}, groups...), a.Key)
		}
		for _, ga := range a.Value.Group() {
			out = appendFlat(out, inner, ga)
		}
		return out
	}
	key := a.Key
	if len(groups) > 0 {
		key = strings.Join(groups, ".") + "." + a.Key
	}
	return append(out, field{key: key, value: a.Value})
}

// nest writes the bound attributes and the record's attributes into dst,
// creating one nested object per group.
func (s handlerState) nest(dst map[string]any, r slog.Record) {
	for _, b := range s.attrs {
		putNested(dst, b.groups, b.attr)
	}
	r.Attrs(func(a slog.Attr) bool {
		putNested(dst, s.groups, a)
		return true
	})
}

func putNested(dst map[string]any, groups []string, a slog.Attr) {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return
	}
	target := dst
	for _, g := range groups {
		child, ok := target[g].(map[string]any)
		if !ok {
			child = map[string]any{}
			target[g] = child
		}
		target = child
	}
	if a.Value.Kind() == slog.KindGroup {
		inner := []string{}
		if a.Key != "" {
			inner = []string{a.Key}
		}
		for _, ga := range a.Value.Group() {
			putNested(target, inner, ga)
		}
		return
	}
	target[a.Key] = a.Value.Any()
}
