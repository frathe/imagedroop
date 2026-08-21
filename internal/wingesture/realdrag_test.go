package wingesture

import "testing"

// Recorded window positions from real title-bar drags on macOS, captured
// with a throwaway probe while a person swirled the window by hand. They
// are the package's ground truth: every synthesised path in
// detector_test.go is a mathematician's idea of a spiral, whereas these are
// what the thresholds actually have to cope with - uneven sample spacing, a
// wobbly centre, and the ~10 Hz ceiling the OS reports window moves at
// during a drag (polling faster returns duplicate coordinates, which is why
// Detector dedupes).

// cwInwardDrag is 17 real samples spanning 1.6s.
var cwInwardDrag = []sample{
	{ms: 0, x: 755, y: 275},
	{ms: 96, x: 796, y: 305},
	{ms: 203, x: 826, y: 369},
	{ms: 298, x: 825, y: 414},
	{ms: 412, x: 761, y: 433},
	{ms: 513, x: 676, y: 423},
	{ms: 619, x: 657, y: 389},
	{ms: 715, x: 685, y: 344},
	{ms: 830, x: 750, y: 346},
	{ms: 933, x: 753, y: 382},
	{ms: 1035, x: 745, y: 405},
	{ms: 1133, x: 719, y: 401},
	{ms: 1246, x: 727, y: 368},
	{ms: 1355, x: 764, y: 374},
	{ms: 1451, x: 751, y: 388},
	{ms: 1547, x: 739, y: 383},
	{ms: 1650, x: 739, y: 380},
}

// ccwInwardDrag is 25 real samples spanning 2.5s.
var ccwInwardDrag = []sample{
	{ms: 0, x: 742, y: 338},
	{ms: 99, x: 733, y: 310},
	{ms: 199, x: 687, y: 299},
	{ms: 299, x: 639, y: 308},
	{ms: 417, x: 600, y: 330},
	{ms: 513, x: 571, y: 395},
	{ms: 611, x: 578, y: 444},
	{ms: 738, x: 627, y: 486},
	{ms: 833, x: 725, y: 484},
	{ms: 932, x: 770, y: 447},
	{ms: 1044, x: 783, y: 373},
	{ms: 1169, x: 754, y: 329},
	{ms: 1265, x: 706, y: 322},
	{ms: 1362, x: 660, y: 340},
	{ms: 1466, x: 638, y: 369},
	{ms: 1585, x: 640, y: 402},
	{ms: 1681, x: 652, y: 419},
	{ms: 1776, x: 672, y: 420},
	{ms: 1876, x: 689, y: 409},
	{ms: 2000, x: 683, y: 386},
	{ms: 2099, x: 675, y: 383},
	{ms: 2212, x: 666, y: 400},
	{ms: 2311, x: 668, y: 420},
	{ms: 2417, x: 669, y: 421},
	{ms: 2538, x: 670, y: 421},
}

// shortDrag is 19 real samples spanning 1.9s.
var shortDrag = []sample{
	{ms: 0, x: 717, y: 390},
	{ms: 114, x: 764, y: 389},
	{ms: 217, x: 820, y: 398},
	{ms: 320, x: 869, y: 420},
	{ms: 432, x: 888, y: 451},
	{ms: 530, x: 887, y: 490},
	{ms: 640, x: 854, y: 519},
	{ms: 736, x: 811, y: 528},
	{ms: 832, x: 768, y: 516},
	{ms: 947, x: 705, y: 453},
	{ms: 1047, x: 701, y: 423},
	{ms: 1151, x: 751, y: 406},
	{ms: 1248, x: 795, y: 436},
	{ms: 1347, x: 771, y: 471},
	{ms: 1447, x: 749, y: 463},
	{ms: 1568, x: 783, y: 438},
	{ms: 1664, x: 778, y: 459},
	{ms: 1764, x: 771, y: 459},
	{ms: 1888, x: 771, y: 458},
}

func TestRealClockwiseInwardDragFires(t *testing.T) {
	d := New(Config{})

	got := feed(d, cwInwardDrag)

	if !got.Detected {
		t.Fatal("the recorded clockwise inward drag should fire")
	}
	if got.Direction != Clockwise {
		t.Errorf("Direction = %v, want Clockwise", got.Direction)
	}
	if !got.Inward {
		t.Error("Inward = false, want true: the recorded drag spirals inward")
	}
}

// Both recorded spirals happen to tighten - people seem to draw them that
// way - so the outward case is covered by the synthesised paths only.
func TestRealCounterClockwiseInwardDragFires(t *testing.T) {
	d := New(Config{})

	got := feed(d, ccwInwardDrag)

	if !got.Detected {
		t.Fatal("the recorded counter-clockwise drag should fire")
	}
	if got.Direction != CounterClockwise {
		t.Errorf("Direction = %v, want CounterClockwise", got.Direction)
	}
	if !got.Inward {
		t.Error("Inward = false, want true: mean radius falls from 97px to 24px across the recording")
	}
}

// The third recorded drag looks spiral-ish to the eye but only completes
// 0.8 of a turn, which is exactly the sort of near-miss the turns threshold
// exists to reject. If a change to the defaults ever makes this one fire,
// ordinary window shuffling will start opening the easter egg too.
func TestRealShortDragDoesNotFire(t *testing.T) {
	d := New(Config{})

	if got := feed(d, shortDrag); got.Detected {
		t.Errorf("the recorded 0.8-turn drag fired: %+v", got)
	}
}
