package service

import (
	"encoding/json"
	"slices"
)

// The run classes of durable spec 1.4. verification is a task an agent runs to check
// an outcome; shadow is a task a shadow agent runs beside the seat's own (F8). Both are
// held by a budget and by a paused company, like routine and delegated work, and
// verification is never checked in turn (no recursion).
const (
	RunClassVerification RunClass = "verification"
	RunClassShadow       RunClass = "shadow"
)

// heldRunClasses is the set of run classes a hold budget and a paused company stop.
// Human and exempt runs are never in it.
func heldRunClasses() map[RunClass]bool {
	return map[RunClass]bool{
		RunClassRoutine:      true,
		RunClassDelegated:    true,
		RunClassVerification: true,
		RunClassShadow:       true,
	}
}

// durableRunClassOf classifies a task that carries a durable marker in its context. It
// returns "" for every other task, which is then classified as before.
func durableRunClassOf(taskContext []byte) RunClass {
	var marker struct {
		DurableKind string `json:"durable_kind"`
	}
	if len(taskContext) == 0 || json.Unmarshal(taskContext, &marker) != nil {
		return ""
	}
	switch marker.DurableKind {
	case "verification":
		return RunClassVerification
	case "shadow":
		return RunClassShadow
	}
	return ""
}

// budgetHoldsClass reports whether a budget's classes hold class. The verification and
// shadow classes were added after budgets existed, and a migration cannot update the
// budget table (its number sorts before the table's creation), so a budget that lists
// routine holds these two as well. New budgets list all four explicitly.
func budgetHoldsClass(classes []string, class RunClass) bool {
	if slices.Contains(classes, string(class)) {
		return true
	}
	return (class == RunClassVerification || class == RunClassShadow) && slices.Contains(classes, string(RunClassRoutine))
}
