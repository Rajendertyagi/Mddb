// Package sliceutil provides small, dependency-free slice helpers shared across
// the daemon and its internal packages.
package sliceutil

// Unique returns a new slice containing the elements of in with duplicates
// removed, preserving first-seen order. The input is not modified.
func Unique[T comparable](in []T) []T {
	seen := make(map[T]struct{}, len(in))
	out := make([]T, 0, len(in))
	for _, x := range in {
		if _, ok := seen[x]; !ok {
			seen[x] = struct{}{}
			out = append(out, x)
		}
	}
	return out
}
