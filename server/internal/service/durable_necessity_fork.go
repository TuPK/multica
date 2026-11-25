package service

// Comparison is the outcome of one shadow pair (spec 10.2).
type Comparison string

const (
	ComparisonPrimary Comparison = "primary"
	ComparisonShadow  Comparison = "shadow"
	ComparisonTie     Comparison = "tie"
	ComparisonUnable  Comparison = "unable"
)

// NecessityVerdict applies spec 10.2 to the pairs of one necessity test. "primary" counts
// as a loss for the skill under test (the primary won), "shadow" as a win, "tie" counts
// toward n but neither way, and "unable" is excluded from n.
func NecessityVerdict(comparisons []Comparison, margin float64, minPairs int) string {
	var n, losses, wins int
	for _, c := range comparisons {
		switch c {
		case ComparisonPrimary:
			losses++
		case ComparisonShadow:
			wins++
		case ComparisonTie:
		default:
			continue
		}
		n++
	}
	if n < minPairs {
		return "running"
	}
	lo, hi := NewcombeDiffCI(losses, wins, n)
	switch {
	case hi < margin:
		return "not_needed"
	case lo > 0:
		return "needed"
	case n >= 2*minPairs:
		return "inconclusive"
	}
	return "running"
}
