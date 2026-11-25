package service

import "sort"

// JurorCandidate is an agent that may sit on a jury, with the runtime provider
// and model it runs on. Diversity is judged on these fields, never on the agent.
type JurorCandidate struct {
	AgentID  string
	Provider string
	Model    string
}

// PickJurors chooses up to size jurors to check work done by doer. doer is never
// picked. Candidates are taken in AgentID order, in three passes: first a provider
// other than doer's and not yet used by the panel, then a model other than doer's
// and not yet used, then whoever is left. So the same pool always gives the same
// panel, however it is ordered.
//
// diversity is the weakest level on the panel: "provider" when no juror shares
// doer's provider, "model_only" when some juror shares the provider but none shares
// the model, "none" when a juror runs doer's exact model or the panel is empty.
func PickJurors(pool []JurorCandidate, doer JurorCandidate, size int) ([]JurorCandidate, string) {
	candidates := make([]JurorCandidate, 0, len(pool))
	for _, c := range pool {
		if c.AgentID != doer.AgentID {
			candidates = append(candidates, c)
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].AgentID < candidates[j].AgentID
	})

	jurors := []JurorCandidate{}
	chosen := map[string]bool{}
	usedProviders := map[string]bool{}
	usedModels := map[string]bool{}
	passes := []func(JurorCandidate) bool{
		func(c JurorCandidate) bool { return c.Provider != doer.Provider && !usedProviders[c.Provider] },
		func(c JurorCandidate) bool { return c.Model != doer.Model && !usedModels[c.Model] },
		func(JurorCandidate) bool { return true },
	}
	for _, eligible := range passes {
		for _, c := range candidates {
			if len(jurors) >= size {
				break
			}
			if chosen[c.AgentID] || !eligible(c) {
				continue
			}
			chosen[c.AgentID] = true
			usedProviders[c.Provider] = true
			usedModels[c.Model] = true
			jurors = append(jurors, c)
		}
	}
	sort.SliceStable(jurors, func(i, j int) bool {
		return jurors[i].AgentID < jurors[j].AgentID
	})

	if len(jurors) == 0 {
		return jurors, "none"
	}
	diversity := "provider"
	for _, j := range jurors {
		if j.Provider != doer.Provider {
			continue
		}
		if j.Model == doer.Model {
			return jurors, "none"
		}
		diversity = "model_only"
	}
	return jurors, diversity
}
