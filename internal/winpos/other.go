//go:build !darwin && !windows && !linux

package winpos

// platformPosition has no implementation on BSD, mobile, wasm, and any
// other backend without a native handle Get can read a position out of;
// Get's caller already treats ok=false as "nothing to save".
func platformPosition(ctx any) (x, y int, ok bool) {
	return 0, 0, false
}

// platformMaximize has no implementation on these backends either - see
// platformPosition above. Maximize's caller treats this as a silent no-op.
func platformMaximize(ctx any) {}

// platformUnmaximize has no implementation on these backends either - see
// platformPosition above. Unmaximize's caller treats this as a silent
// no-op.
func platformUnmaximize(ctx any) {}
