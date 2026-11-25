package issueworkflow

// EffectsExternal is the effects value of a task whose work has effects outside the
// platform. The platform holds such a task until a person approves it (durable spec
// 5.4); the gate that starts the request is not changed by it.
const EffectsExternal = "external"

// checkEffects is T09: a task's effects is empty or external, and no other value.
func (v *validator) checkEffects(tp string, t DocTask) {
	switch t.Effects {
	case "", EffectsExternal:
	default:
		v.errp("T09", "effects_invalid", tp+".effects", map[string]any{"effects": t.Effects},
			"effects is external or unset, not %q", t.Effects)
	}
}
