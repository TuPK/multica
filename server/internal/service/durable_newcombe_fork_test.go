package service

import (
	"math"
	"testing"
)

func round3(x float64) float64 { return math.Round(x*1000) / 1000 }

// TestNewcombeKnownValues checks NewcombeDiffCI against values worked out by hand.
//
// Wilson score bounds for x successes in n, z = 1.959964:
//
//	centre = (p + z²/2n) / (1 + z²/n)
//	half   = z·sqrt(p(1−p)/n + z²/4n²) / (1 + z²/n)
//
// Paired hybrid for d = pL − pW on the same n pairs (Newcombe 1998, matched pairs):
//
//	lo = d − sqrt(X² + Y² − 2φXY),   X = pL − lL, Y = uW − pW
//	hi = d + sqrt(X'² + Y'² − 2φX'Y'), X' = uL − pL, Y' = pW − lW
//	φ  = −LW / sqrt(L(W+T) · W(L+T)), T = n − L − W (0 when L = 0 or W = 0)
func TestNewcombeKnownValues(t *testing.T) {
	cases := []struct {
		name           string
		lossA, winA, n int
		lo, hi         float64
	}{
		{
			// n=10, L=W=0: p=0, Wilson [0, 3.8415/13.8415] = [0, 0.2775]. φ=0, d=0.
			// hi = 0.2775 − 0, lo = −0.2775.
			name:  "no decisive pairs, n=10",
			lossA: 0, winA: 0, n: 10,
			lo: -0.278, hi: 0.278,
		},
		{
			// n=40, L=12, W=0, T=28: d=0.3. Wilson 12/40 = [0.1807, 0.4543]; Wilson 0/40 = [0, 0.0876].
			// φ=0 because W=0. lo = 0.3 − sqrt(0.1193² + 0.0876²) = 0.3 − 0.1480 = 0.152.
			// hi = 0.3 + sqrt(0.1543² + 0²) = 0.4543.
			name:  "one-sided losses, n=40",
			lossA: 12, winA: 0, n: 40,
			lo: 0.152, hi: 0.454,
		},
		{
			// n=20, L=W=10, T=0: d=0. Wilson 10/20 = [0.2993, 0.7007], half 0.2007.
			// φ = −100 / sqrt(10·10·10·10) = −1 (every pair is loss or win, so they are complementary).
			// lo = −sqrt(0.2007² + 0.2007² + 2·0.2007²) = −0.4014, and hi = +0.4014.
			// The independent-samples Newcombe formula would give ±0.284 here, which is too narrow.
			name:  "complementary outcomes, n=20",
			lossA: 10, winA: 10, n: 20,
			lo: -0.401, hi: 0.401,
		},
		{
			// n=20, L=3, W=5, T=12: φ = −15/sqrt(3·17·5·15) = −0.2425.
			// Wilson 3/20 = [0.0524, 0.3604], Wilson 5/20 = [0.1119, 0.4687], d = −0.10.
			// lo = −0.10 − sqrt(0.0976² + 0.2187² + 2·0.2425·0.0976·0.2187) = −0.3602.
			// hi = −0.10 + sqrt(0.2104² + 0.1381² + 2·0.2425·0.2104·0.1381) = +0.1783.
			name:  "mixed outcomes, n=20",
			lossA: 3, winA: 5, n: 20,
			lo: -0.360, hi: 0.178,
		},
		{
			// n=40, ties only: upper bound 3.8415/43.8415 = 0.0876. This is the margin problem in
			// spec 10.5 item 2: at n=40 a 95% interval cannot get below 0.05.
			name:  "all ties, n=40",
			lossA: 0, winA: 0, n: 40,
			lo: -0.088, hi: 0.088,
		},
		{
			// n=80, ties only: upper bound 3.8415/83.8415 = 0.0458, which is below 0.05.
			name:  "all ties, n=80",
			lossA: 0, winA: 0, n: 80,
			lo: -0.046, hi: 0.046,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lo, hi := NewcombeDiffCI(tc.lossA, tc.winA, tc.n)
			if got := round3(lo); got != tc.lo {
				t.Errorf("lo = %.3f, want %.3f", got, tc.lo)
			}
			if got := round3(hi); got != tc.hi {
				t.Errorf("hi = %.3f, want %.3f", got, tc.hi)
			}
		})
	}
}
