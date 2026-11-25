package service

import "math"

// z95 is the two-sided 95% normal quantile.
const z95 = 1.959963984540054

// wilsonScore returns the Wilson score interval for x successes in n trials.
func wilsonScore(x, n int) (lo, hi float64) {
	nf := float64(n)
	p := float64(x) / nf
	z2 := z95 * z95
	centre := (p + z2/(2*nf)) / (1 + z2/nf)
	half := z95 * math.Sqrt(p*(1-p)/nf+z2/(4*nf*nf)) / (1 + z2/nf)
	return math.Max(0, centre-half), math.Min(1, centre+half)
}

// NewcombeDiffCI returns the 95% interval for d = pLoss − pWin, where lossA and winA
// are counts of two mutually exclusive outcomes among the same n pairs.
//
// Newcombe's hybrid (Wilson per proportion) is derived for two independent samples.
// Here both proportions come from one multinomial, so the terms are correlated, and
// ignoring that (φ = 0) makes the interval too narrow: at n=20 with 10 losses and 10
// wins it gives ±0.284 where the matched-pairs form gives ±0.401. The correlation is
// the phi coefficient of the two indicators on the same pairs (Newcombe 1998, method 10):
// φ = −LW / sqrt(L(W+T)·W(L+T)), with T = n − L − W. It is 0 when L = 0 or W = 0.
// The interval is still an approximation, not an exact multinomial interval.
//
// n ≤ 0 returns the full range (−1, 1).
func NewcombeDiffCI(lossA, winA, n int) (lo, hi float64) {
	if n <= 0 {
		return -1, 1
	}
	ties := n - lossA - winA
	var phi float64
	if lossA > 0 && winA > 0 {
		phi = -float64(lossA*winA) / math.Sqrt(float64(lossA*(winA+ties))*float64(winA*(lossA+ties)))
	}
	nf := float64(n)
	pL := float64(lossA) / nf
	pW := float64(winA) / nf
	lL, uL := wilsonScore(lossA, n)
	lW, uW := wilsonScore(winA, n)

	d := pL - pW
	x, y := pL-lL, uW-pW
	xu, yu := uL-pL, pW-lW
	lo = d - math.Sqrt(math.Max(0, x*x+y*y-2*phi*x*y))
	hi = d + math.Sqrt(math.Max(0, xu*xu+yu*yu-2*phi*xu*yu))
	return lo, hi
}
