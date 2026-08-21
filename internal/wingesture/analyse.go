package wingesture

import "math"

// analyse decides whether pts trace a spiral. It is the whole of the
// recognition, kept as a function over a slice so it can be reasoned about
// - and tested - without a Detector, a clock, or a window anywhere near it.
//
// The steps, in order:
//
//  1. Take the centroid as the spiral's centre. A drawn spiral wanders, so
//     there is no better centre available than the one the path itself
//     averages to.
//  2. Walk the path accumulating the shortest-arc angle change about that
//     centre. The total is the signed turn count; its sign is the direction
//     (positive is clockwise - see the package doc on y-down coordinates).
//  3. Track how many steps agree with the majority sign. A drawn spiral is
//     almost perfectly consistent; noise is not.
//  4. Fit the radius against angular progress. A spiral's radius trends,
//     a circle's does not - this is the only thing separating the two.
//
// Progress in step 4 is accumulated *unsigned*, so the fitted slope means
// the same thing in both directions: negative is always inward. Fitting
// against the signed angle instead would invert the slope for every
// counter-clockwise gesture, reporting an opening spiral as a tightening
// one.
func analyse(pts []point, cfg Config) (Result, bool) {
	if len(pts) < 2 {
		return Result{}, false
	}

	cx, cy := centroid(pts)

	var (
		total     float64 // signed angle swept
		progress  float64 // unsigned angle swept, the spiral's own arc length
		meanR     float64
		agree     int
		against   int
		prevTheta float64
		havePrev  bool
	)

	// Radii paired with the progress at which they were reached, for the
	// least-squares fit below.
	radii := make([]float64, 0, len(pts))
	progs := make([]float64, 0, len(pts))

	for _, p := range pts {
		dx := float64(p.x) - cx
		dy := float64(p.y) - cy
		r := math.Hypot(dx, dy)
		meanR += r

		// A point sitting exactly on the centre has no meaningful angle;
		// skipping it is better than letting atan2(0, 0)'s zero masquerade
		// as a real reading and inject a phantom step.
		if r == 0 {
			continue
		}

		theta := math.Atan2(dy, dx)
		if havePrev {
			delta := wrap(theta - prevTheta)
			total += delta
			progress += math.Abs(delta)
			switch {
			case delta > 0:
				agree++
			case delta < 0:
				against++
			}
			radii = append(radii, r)
			progs = append(progs, progress)
		}
		prevTheta, havePrev = theta, true
	}

	meanR /= float64(len(pts))
	if meanR < cfg.MinMeanRadius {
		return Result{}, false
	}

	if math.Abs(total)/(2*math.Pi) < cfg.MinTurns {
		return Result{}, false
	}

	steps := agree + against
	if steps == 0 {
		return Result{}, false
	}
	if float64(max(agree, against))/float64(steps) < cfg.MinConsistency {
		return Result{}, false
	}

	slope, ok := fitSlope(progs, radii)
	if !ok {
		return Result{}, false
	}
	// The fit is radius per radian; a turn is 2*pi of them.
	trendPerTurn := slope * 2 * math.Pi
	if math.Abs(trendPerTurn) < cfg.MinRadiusTrend*meanR {
		return Result{}, false
	}

	dir := Clockwise
	if total < 0 {
		dir = CounterClockwise
	}

	return Result{Detected: true, Direction: dir, Inward: trendPerTurn < 0}, true
}

// centroid averages the path's own points, which is as good a centre as a
// hand-drawn spiral offers.
func centroid(pts []point) (cx, cy float64) {
	for _, p := range pts {
		cx += float64(p.x)
		cy += float64(p.y)
	}
	n := float64(len(pts))
	return cx / n, cy / n
}

// wrap folds an angle difference into (-pi, pi], so travelling past the
// atan2 discontinuity reads as the short way round rather than a full
// reverse revolution.
func wrap(d float64) float64 {
	for d > math.Pi {
		d -= 2 * math.Pi
	}
	for d <= -math.Pi {
		d += 2 * math.Pi
	}
	return d
}

// fitSlope is the least-squares gradient of ys against xs, reporting false
// when xs carry no spread for a gradient to be measured across.
func fitSlope(xs, ys []float64) (float64, bool) {
	if len(xs) < 2 || len(xs) != len(ys) {
		return 0, false
	}

	var mx, my float64
	for i := range xs {
		mx += xs[i]
		my += ys[i]
	}
	n := float64(len(xs))
	mx /= n
	my /= n

	var num, den float64
	for i := range xs {
		dx := xs[i] - mx
		num += dx * (ys[i] - my)
		den += dx * dx
	}
	if den == 0 {
		return 0, false
	}

	return num / den, true
}
