package service

import (
	"fmt"

	"github.com/multica-ai/multica/server/internal/connectedapps"
)

// Override is a workspace's saved decision for one cell of the effect policy
// table (spec 4.3). Only the first matching override for a cell applies.
type Override struct {
	AutonomyLevel int
	Effect        string
	Reversibility string
	Decision      string
}

// Run classes and company states the policy knows about.
const (
	runClassHuman        = "human"
	runClassShadow       = "shadow"
	runClassVerification = "verification"
	companyPaused        = "paused"
)

// sendDailyCap is how many sends an L3 agent may make per day before further
// sends rehearse instead.
const sendDailyCap = 20

// effectPolicyTable is spec 4.3. Columns are autonomy levels L0..L3. Rows are
// keyed by effectRow: the effect class, or for write the class plus its
// reversibility.
var effectPolicyTable = map[string][4]string{
	"read":              {"allow", "allow", "allow", "allow"},
	"write_internal":    {"allow", "allow", "allow", "allow"},
	"write/reversible":  {"confirm", "allow", "allow", "allow"},
	"write/compensable": {"confirm", "confirm", "allow", "allow"},
	"send":              {"block", "rehearse", "rehearse", "allow"},
	"share":             {"block", "rehearse", "rehearse", "rehearse"},
	"delete":            {"block", "confirm", "rehearse", "rehearse"},
	"spend":             {"block", "rehearse", "rehearse", "rehearse"},
}

// effectRow names the table row for an effect and reversibility. Reversibility
// only matters for write; the other classes have a single row. It reports false
// when the table has no row for them, and callers treat that as block.
func effectRow(effect, rev string) (string, bool) {
	switch effect {
	case connectedapps.EffectRead, connectedapps.EffectWriteInternal,
		connectedapps.EffectSend, connectedapps.EffectShare,
		connectedapps.EffectDelete, connectedapps.EffectSpend:
		return effect, true
	case connectedapps.EffectWrite:
		switch rev {
		case connectedapps.ReversibilityReversible:
			return "write/reversible", true
		case connectedapps.ReversibilityCompensable:
			return "write/compensable", true
		}
	}
	return "", false
}

// baseDecision is the table's decision for one cell. It reports false when the
// cell does not exist.
func baseDecision(level int, effect, rev string) (string, bool) {
	row, ok := effectRow(effect, rev)
	if !ok || level < 0 || level > 3 {
		return "", false
	}
	return effectPolicyTable[row][level], true
}

func validDecision(decision string) bool {
	switch decision {
	case connectedapps.PolicyAllow, connectedapps.PolicyConfirm,
		connectedapps.PolicyRehearse, connectedapps.PolicyBlock:
		return true
	}
	return false
}

// isLooser reports whether decision lets an action through more freely than base.
func isLooser(decision, base string) bool {
	return connectedapps.Stricter(base, decision) != decision
}

// findOverride returns the override saved for the same cell, if any.
func findOverride(overrides []Override, level int, effect, rev string) (Override, bool) {
	row, ok := effectRow(effect, rev)
	if !ok {
		return Override{}, false
	}
	for _, o := range overrides {
		if o.AutonomyLevel != level {
			continue
		}
		if r, ok := effectRow(o.Effect, o.Reversibility); ok && r == row {
			return o, true
		}
	}
	return Override{}, false
}

// CellOverride returns the decision the workspace saved for one cell, if any.
func CellOverride(overrides []Override, level int, effect, rev string) (string, bool) {
	o, ok := findOverride(overrides, level, effect, rev)
	return o.Decision, ok
}

// ValidateOverride reports whether a workspace may save decision for one cell.
// Stricter decisions are always allowed. Looser ones are allowed only at L3,
// and spend and share may never become allow.
func ValidateOverride(level int, effect, rev, decision string) error {
	base, ok := baseDecision(level, effect, rev)
	if !ok {
		return fmt.Errorf("no policy cell for L%d, effect %q, reversibility %q", level, effect, rev)
	}
	if !validDecision(decision) {
		return fmt.Errorf("decision %q is not one of allow, confirm, rehearse, block", decision)
	}
	if decision == connectedapps.PolicyAllow && (effect == connectedapps.EffectSpend || effect == connectedapps.EffectShare) {
		return fmt.Errorf("%s cannot be overridden to allow at any level", effect)
	}
	if !isLooser(decision, base) {
		return nil
	}
	if level != 3 {
		return fmt.Errorf("default %s at L%d is %s; loosening to %s is allowed only at L3", effect, level, base, decision)
	}
	return nil
}

// DecideEffect returns allow, confirm, rehearse or block for one action.
//
// It starts from the table cell, applies the workspace's valid override for the
// cell, then limits L3 sends to sendDailyCap per day, and finally blocks
// anything the task class or a paused company forbids. Overrides that
// ValidateOverride rejects are ignored, so the default stands.
func DecideEffect(level int, effect, rev string, overrides []Override, runClass, companyState string, sendCountToday int) string {
	decision, ok := baseDecision(level, effect, rev)
	if !ok {
		return connectedapps.PolicyBlock
	}
	if o, ok := findOverride(overrides, level, effect, rev); ok && ValidateOverride(level, effect, rev, o.Decision) == nil {
		decision = o.Decision
	}
	if decision == connectedapps.PolicyAllow && level == 3 && effect == connectedapps.EffectSend && sendCountToday >= sendDailyCap {
		decision = connectedapps.PolicyRehearse
	}
	if taskClassForbids(runClass, effect) {
		return connectedapps.PolicyBlock
	}
	if companyState == companyPaused && runClass != runClassHuman && effect != connectedapps.EffectRead {
		return connectedapps.PolicyBlock
	}
	return decision
}

// taskClassForbids reports whether a task class may not perform effect at all.
// Shadow runs read only; verification runs read and write_internal only.
func taskClassForbids(runClass, effect string) bool {
	switch runClass {
	case runClassShadow:
		return effect != connectedapps.EffectRead
	case runClassVerification:
		return effect != connectedapps.EffectRead && effect != connectedapps.EffectWriteInternal
	}
	return false
}

// Lock codes name why a workspace may not save a decision to a cell. The policy grid
// shows the code beside the locked option; LockNone means the decision can be saved.
const (
	LockNone       = ""
	LockNoCell     = "no_cell"
	LockInvalid    = "invalid_decision"
	LockNeverAllow = "never_allow"
	LockLooserAtL3 = "loosen_needs_l3"
)

// OverrideLock returns the lock code for saving decision to one cell. It agrees with
// ValidateOverride: a decision is locked exactly when ValidateOverride refuses it.
func OverrideLock(level int, effect, rev, decision string) string {
	if _, ok := baseDecision(level, effect, rev); !ok {
		return LockNoCell
	}
	if !validDecision(decision) {
		return LockInvalid
	}
	if decision == connectedapps.PolicyAllow && (effect == connectedapps.EffectSpend || effect == connectedapps.EffectShare) {
		return LockNeverAllow
	}
	if ValidateOverride(level, effect, rev, decision) != nil {
		return LockLooserAtL3
	}
	return LockNone
}

// PolicyRow is one row of the effect policy table as the policy grid shows it. Only
// write has two rows, one per reversibility; the other classes ignore reversibility, so
// each is stored under the reversibility that suits it.
type PolicyRow struct {
	Effect        string
	Reversibility string
}

// PolicyRows lists the table's rows in the order the grid shows them.
func PolicyRows() []PolicyRow {
	return []PolicyRow{
		{connectedapps.EffectRead, connectedapps.ReversibilityReversible},
		{connectedapps.EffectWriteInternal, connectedapps.ReversibilityReversible},
		{connectedapps.EffectWrite, connectedapps.ReversibilityReversible},
		{connectedapps.EffectWrite, connectedapps.ReversibilityCompensable},
		{connectedapps.EffectSend, connectedapps.ReversibilityIrreversible},
		{connectedapps.EffectShare, connectedapps.ReversibilityIrreversible},
		{connectedapps.EffectDelete, connectedapps.ReversibilityIrreversible},
		{connectedapps.EffectSpend, connectedapps.ReversibilityIrreversible},
	}
}
