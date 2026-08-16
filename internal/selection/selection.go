// Package selection is the multi-select model behind the grid overview's
// batch actions: which files are picked, and the anchor a range extension
// measures from.
//
// It is deliberately just integers. What an index means - a position in the
// app's file set, not a position in the grid - and which gesture calls which
// method are internal/ui/grid's business; this package draws nothing and
// imports no fyne at all, which is why it sits here beside internal/filesort
// rather than under internal/ui.
package selection

import (
	"maps"
	"slices"
)

// Set is a set of file indices plus the anchor a range extension measures
// from. The zero value is not usable - see New.
type Set struct {
	members map[int]struct{}

	// anchor is the last index a Toggle touched, and hasAnchor whether one
	// ever has. Kept separate from the membership: a Shift+click extends
	// from wherever the last Cmd+click landed whether that click selected
	// or deselected the cell, so an anchor routinely names an index the
	// set no longer holds.
	anchor    int
	hasAnchor bool
}

// New builds an empty set.
func New() *Set {
	return &Set{members: make(map[int]struct{})}
}

// Toggle flips whether i is in the set, and makes i the anchor either way.
// Reports whether i is in the set afterwards.
func (s *Set) Toggle(i int) bool {
	s.anchor = i
	s.hasAnchor = true

	if _, ok := s.members[i]; ok {
		delete(s.members, i)
		return false
	}

	s.members[i] = struct{}{}

	return true
}

// Add puts i in the set without moving the anchor - what a range extension
// walks with, since the anchor is the one end of that range that must not
// move. A no-op when i is already in.
func (s *Set) Add(i int) {
	s.members[i] = struct{}{}
}

// Contains reports whether i is in the set.
func (s *Set) Contains(i int) bool {
	_, ok := s.members[i]

	return ok
}

// Len is how many indices are in the set.
func (s *Set) Len() int {
	return len(s.members)
}

// Clear empties the set and drops the anchor with it: there is nothing left
// for a range to extend from.
func (s *Set) Clear() {
	clear(s.members)
	s.anchor = 0
	s.hasAnchor = false
}

// Replace swaps the membership for is, leaving the anchor alone - so a
// select-all followed by a Shift+click still extends from wherever the user
// last clicked.
func (s *Set) Replace(is []int) {
	clear(s.members)
	for _, i := range is {
		s.members[i] = struct{}{}
	}
}

// Indices is the set in ascending order, as a slice the caller owns: the
// batch actions hold their target list across a background trash move, so it
// must not alias state a later keystroke can mutate.
func (s *Set) Indices() []int {
	return slices.Sorted(maps.Keys(s.members))
}

// Anchor is the index a range extension measures from, ok=false when no
// Toggle has happened since the last Clear.
func (s *Set) Anchor() (int, bool) {
	return s.anchor, s.hasAnchor
}

// Range is the inclusive span between a and b, always ascending however the
// two are ordered - a Shift+click can name a cell either side of the anchor.
func Range(a, b int) []int {
	if a > b {
		a, b = b, a
	}

	span := make([]int, 0, b-a+1)
	for i := a; i <= b; i++ {
		span = append(span, i)
	}

	return span
}
