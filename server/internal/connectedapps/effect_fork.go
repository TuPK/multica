package connectedapps

// Effect classes: what an action does, as the platform sees it. Apps map to
// the first five through MapAppClass; write_internal and spend come from
// elsewhere (workspace writes and the curated list or a declaration).
const (
	EffectRead          = "read"
	EffectWriteInternal = "write_internal"
	EffectWrite         = "write"
	EffectSend          = "send"
	EffectShare         = "share"
	EffectDelete        = "delete"
	EffectSpend         = "spend"
)

// Reversibility: whether an effect can be taken back.
const (
	ReversibilityReversible   = "reversible"
	ReversibilityCompensable  = "compensable"
	ReversibilityIrreversible = "irreversible"
)

// PolicyRehearse is a decision that runs the action as a rehearsal instead of
// performing it. It is never a rule an account can save (ValidPolicy rejects it).
const PolicyRehearse = "rehearse"

// MapAppClass maps an app action class to its effect class, or "" when the
// class is not one an app can produce.
func MapAppClass(class string) string {
	switch class {
	case ClassRead:
		return EffectRead
	case ClassWrite:
		return EffectWrite
	case ClassSend:
		return EffectSend
	case ClassDelete:
		return EffectDelete
	case ClassShare:
		return EffectShare
	}
	return ""
}

// DefaultReversibility is the reversibility assumed for an effect when no
// curated tool says otherwise. Read has nothing to reverse and returns "".
func DefaultReversibility(effect string) string {
	switch effect {
	case EffectWriteInternal:
		return ReversibilityReversible
	case EffectWrite:
		return ReversibilityCompensable
	case EffectSend, EffectShare, EffectDelete, EffectSpend:
		return ReversibilityIrreversible
	}
	return ""
}

// strictnessFork ranks the policies added for effect decisions, extending the
// order allow < confirm < rehearse < block. Unknown policies rank as block.
func strictnessFork(policy string) int {
	switch policy {
	case PolicyRehearse:
		return 2
	default:
		return 3
	}
}
