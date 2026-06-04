package saft

import (
	"cmp"
	"maps"
	"slices"
)

// mapSlice returns nil on empty input, otherwise a slice of f(src[i]) for each
// element. The nil-on-empty contract matches encoding/xml's `omitempty` —
// optional elements drop out of the output when there's nothing to project.
func mapSlice[T, R any](src []T, f func(T) R) []R {
	if len(src) == 0 {
		return nil
	}
	out := make([]R, len(src))
	for i, v := range src {
		out[i] = f(v)
	}
	return out
}

// sortByKey sorts s in place by the string projection of each element.
// Used by every family aggregator to give the export a deterministic order.
func sortByKey[T any](s []T, key func(T) string) {
	slices.SortFunc(s, func(a, b T) int { return cmp.Compare(key(a), key(b)) })
}

// sortedValues returns a map's values as a slice ordered by the projected key.
func sortedValues[K comparable, V any](m map[K]V, key func(V) string) []V {
	out := slices.Collect(maps.Values(m))
	sortByKey(out, key)
	return out
}
