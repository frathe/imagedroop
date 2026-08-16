package selection

import (
	"slices"
	"testing"
)

// These exercise the set directly, without a grid: which gesture calls
// which method - and what a display index means - stays in
// internal/ui/grid, which is what actually owns those behaviours.

func TestNew_IsEmptyAndHasNoAnchor(t *testing.T) {
	s := New()

	if s.Len() != 0 {
		t.Errorf("Len() = %d, want 0", s.Len())
	}
	if got := s.Indices(); len(got) != 0 {
		t.Errorf("Indices() = %v, want empty", got)
	}
	if _, ok := s.Anchor(); ok {
		t.Error("Anchor() ok = true on a fresh set, want false")
	}
}

func TestToggle_AddsThenRemoves(t *testing.T) {
	s := New()

	if added := s.Toggle(3); !added {
		t.Error("Toggle(3) on an empty set = false, want true (added)")
	}
	if !s.Contains(3) {
		t.Error("Contains(3) = false after Toggle(3), want true")
	}

	if added := s.Toggle(3); added {
		t.Error("Toggle(3) on a set already holding 3 = true, want false (removed)")
	}
	if s.Contains(3) {
		t.Error("Contains(3) = true after a second Toggle(3), want false")
	}
}

// TestToggle_MovesAnchorEvenWhenDeselecting pins the gesture Shift+click
// measures from: cmd-clicking a cell makes it the anchor whether that click
// selected or deselected it, the same way a file manager's does.
func TestToggle_MovesAnchorEvenWhenDeselecting(t *testing.T) {
	s := New()

	s.Toggle(2)
	if a, ok := s.Anchor(); !ok || a != 2 {
		t.Errorf("Anchor() = (%d, %v) after Toggle(2), want (2, true)", a, ok)
	}

	s.Toggle(7)
	s.Toggle(7) // deselects it again
	if a, ok := s.Anchor(); !ok || a != 7 {
		t.Errorf("Anchor() = (%d, %v) after deselecting 7, want (7, true)", a, ok)
	}
}

func TestAdd_LeavesTheAnchorWhereItWas(t *testing.T) {
	s := New()
	s.Toggle(4)

	s.Add(9)

	if !s.Contains(9) {
		t.Error("Contains(9) = false after Add(9), want true")
	}
	if a, ok := s.Anchor(); !ok || a != 4 {
		t.Errorf("Anchor() = (%d, %v) after Add(9), want the anchor left at (4, true)", a, ok)
	}
}

// TestAdd_IsIdempotent matters for range extension, which re-adds cells the
// previous range already covered.
func TestAdd_IsIdempotent(t *testing.T) {
	s := New()

	s.Add(1)
	s.Add(1)

	if s.Len() != 1 {
		t.Errorf("Len() = %d after adding 1 twice, want 1", s.Len())
	}
}

func TestClear_EmptiesTheSetAndDropsTheAnchor(t *testing.T) {
	s := New()
	s.Toggle(1)
	s.Add(2)

	s.Clear()

	if s.Len() != 0 {
		t.Errorf("Len() = %d after Clear(), want 0", s.Len())
	}
	if _, ok := s.Anchor(); ok {
		t.Error("Anchor() ok = true after Clear(), want false")
	}
}

func TestReplace_SwapsTheWholeSet(t *testing.T) {
	s := New()
	s.Toggle(1)
	s.Add(2)

	s.Replace([]int{5, 6, 7})

	want := []int{5, 6, 7}
	if got := s.Indices(); !slices.Equal(got, want) {
		t.Errorf("Indices() = %v after Replace, want %v", got, want)
	}
}

// TestReplace_KeepsTheAnchor is what lets Cmd+A followed by Shift+click
// still extend from wherever the user last clicked.
func TestReplace_KeepsTheAnchor(t *testing.T) {
	s := New()
	s.Toggle(4)

	s.Replace([]int{0, 1, 2})

	if a, ok := s.Anchor(); !ok || a != 4 {
		t.Errorf("Anchor() = (%d, %v) after Replace, want (4, true)", a, ok)
	}
}

func TestIndices_IsSortedAscending(t *testing.T) {
	s := New()
	for _, i := range []int{9, 2, 5, 0} {
		s.Add(i)
	}

	want := []int{0, 2, 5, 9}
	if got := s.Indices(); !slices.Equal(got, want) {
		t.Errorf("Indices() = %v, want %v", got, want)
	}
}

// TestIndices_ReturnsAFreshSlice guards the batch actions, which hold the
// returned slice while a trash move runs in the background.
func TestIndices_ReturnsAFreshSlice(t *testing.T) {
	s := New()
	s.Add(1)
	s.Add(2)

	got := s.Indices()
	got[0] = 99

	if want := []int{1, 2}; !slices.Equal(s.Indices(), want) {
		t.Errorf("Indices() = %v after mutating an earlier result, want %v", s.Indices(), want)
	}
}

func TestRange_IsInclusiveAndAscendingWhicheverWayItIsGiven(t *testing.T) {
	tests := []struct {
		name string
		a, b int
		want []int
	}{
		{"forwards", 2, 5, []int{2, 3, 4, 5}},
		{"backwards", 5, 2, []int{2, 3, 4, 5}},
		{"single cell", 3, 3, []int{3}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Range(tt.a, tt.b); !slices.Equal(got, tt.want) {
				t.Errorf("Range(%d, %d) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
