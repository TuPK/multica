package service

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
	"time"
)

// Verification modes for a (agent, work class) pair (durable spec 3.3).
const (
	VerificationModeFull    = "full"
	VerificationModeSampled = "sampled"
	VerificationModeSpot    = "spot"
)

// TrackRow is one track_record row: settled outcome counts for a window,
// with the Wilson lower bound already computed.
type TrackRow struct {
	N, NPass, NPartial, NFail, NHighFail int
	ScoreLower                           float64
	LastHighFailAt                       time.Time
}

// WilsonLower is the lower bound of the 95% Wilson interval (z = 1.96) for the
// pass rate, where a partial counts as half a pass. With no outcomes it is 0.
func WilsonLower(pass, partial, fail int) float64 {
	n := float64(pass + partial + fail)
	if n == 0 {
		return 0
	}
	const z = 1.96
	p := (float64(pass) + 0.5*float64(partial)) / n
	center := p + z*z/(2*n)
	margin := z * math.Sqrt(p*(1-p)/n+z*z/(4*n*n))
	return (center - margin) / (1 + z*z/n)
}

// NextVerificationMode returns the verification mode for the next calculation.
// Any high fail in the 30-day window drops to full at once. Otherwise the mode
// falls one step when its score drops below the bar, and rises one step at most
// once per 7 days when its bar is met.
func NextVerificationMode(cur string, r30, r90 TrackRow, lastChange, now time.Time) string {
	if r30.NHighFail > 0 {
		return VerificationModeFull
	}
	switch cur {
	case VerificationModeSpot:
		if r90.ScoreLower < 0.90 {
			return VerificationModeSampled
		}
		return VerificationModeSpot
	case VerificationModeSampled:
		if r30.ScoreLower < 0.80 {
			return VerificationModeFull
		}
		if now.Sub(lastChange) < 7*24*time.Hour {
			return VerificationModeSampled
		}
		if r90.N >= 30 && r90.ScoreLower >= 0.90 && r90.NHighFail == 0 {
			return VerificationModeSpot
		}
		return VerificationModeSampled
	default:
		if now.Sub(lastChange) < 7*24*time.Hour {
			return VerificationModeFull
		}
		if r30.N >= 10 && r30.ScoreLower >= 0.80 {
			return VerificationModeSampled
		}
		return VerificationModeFull
	}
}

// Sampled reports whether an outcome is verified under mode. The choice is
// deterministic: the first 8 bytes of sha256(outcomeID) as a fraction of 2^64
// are compared with the mode's rate. Unknown modes verify everything.
func Sampled(outcomeID string, mode string) bool {
	var rate float64
	switch mode {
	case VerificationModeSampled:
		rate = 0.30
	case VerificationModeSpot:
		rate = 0.10
	default:
		return true
	}
	sum := sha256.Sum256([]byte(outcomeID))
	v := binary.BigEndian.Uint64(sum[:8])
	return float64(v)/(1<<64) < rate
}
