package litscout

// Decision is the next step the quality gate chooses.
type Decision string

const (
	DecisionSummarize          Decision = "summarize"
	DecisionRefineQuery        Decision = "refine_query"
	DecisionSummarizeAvailable Decision = "summarize_available"
	DecisionStopNoNewQueries   Decision = "stop_no_new_queries"
)

// Route decides from counts alone. A model never chooses the flow.
func Route(selected, minPapers, round, maxRounds int, noNewQueries bool) Decision {
	if selected >= minPapers {
		return DecisionSummarize
	}
	if noNewQueries {
		return DecisionStopNoNewQueries
	}
	if round < maxRounds {
		return DecisionRefineQuery
	}
	return DecisionSummarizeAvailable
}
