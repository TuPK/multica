package issueworkflow

import (
	"fmt"
	"sort"
	"strings"
)

// The role, task, join, fact, message and release rules of the validator (spec
// section 3.3 to 3.6). Structure and graph rules are in flowdoc_validate.go.

// acked reports whether the policy carries an admin acknowledgement of the kind.
func (v *validator) acked(kind string) bool {
	for _, a := range v.doc.Policy.Acks {
		if a.Kind == kind {
			return true
		}
	}
	return false
}

// people are the people who can fill a role: the ones bound to it, or, while it
// is unbound, as many stand-ins as its pool says. Stand-ins of different roles
// are different people.
func (v *validator) people(roleKey string) []string {
	r, declared := v.roles[roleKey]
	if !declared {
		return []string{"builtin:" + roleKey}
	}
	if r.Bind != nil {
		ids := r.Bind.AgentIDs
		if r.Kind == RoleKindMember {
			ids = r.Bind.MemberIDs
		}
		if len(ids) > 0 {
			return ids
		}
	}
	n := r.Pool
	if n < 1 {
		n = 1
	}
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("role:%s#%d", roleKey, i)
	}
	return out
}

// ---------------------------------------------------------------- R: roles

func (v *validator) checkRoles() {
	d := v.doc

	for i, r := range d.Roles {
		rp := rolePath(label(r.Key, i))
		switch r.Kind {
		case RoleKindAgent, RoleKindMember:
		case RoleKindLead, RoleKindRequester, RoleKindManagers:
			if r.Key != r.Kind || r.Bind != nil {
				v.errp("R01", "role_kind_invalid", rp, map[string]any{"role": r.Key, "kind": r.Kind}, "a %s role has the key %q and no bind", r.Kind, r.Kind)
			}
		default:
			v.errp("R01", "role_kind_invalid", rp+".kind", map[string]any{"role": r.Key, "kind": r.Kind}, "role %q: kind must be agent, member, lead, requester or managers", r.Key)
		}
		if r.Pool < 0 || r.Pool > DocMaxInstances {
			v.errp("S02", "limit_exceeded", rp+".pool", map[string]any{"limit": "pool", "max": DocMaxInstances, "actual": r.Pool}, "role %q: pool is 1 to %d", r.Key, DocMaxInstances)
		}
	}

	used := map[string]bool{}
	for i, s := range d.Steps {
		sp := stepPath(label(s.Key, i))
		if s.ExitRole != "" {
			used[s.ExitRole] = true
			if _, ok := v.roles[s.ExitRole]; !ok && !v.isBuiltinRole(s.ExitRole) {
				v.errp("R01", "role_unknown", sp+".exit_role", map[string]any{"role": s.ExitRole}, "exit_role %q is not a role", s.ExitRole)
			}
		}
		for j, t := range s.Tasks {
			tp := taskPath(label(s.Key, i), label(t.Key, j))
			used[t.Role] = true
			if _, ok := v.roles[t.Role]; !ok && !v.isBuiltinRole(t.Role) {
				v.errp("R01", "role_unknown", tp+".role", map[string]any{"role": t.Role}, "task %q: role %q is not a role", t.Key, t.Role)
			}
			// R05
			if v.roleKind(t.Role) == RoleKindLead && !(v.doc.Policy.LeadMayWork && v.acked(AckLeadMayWork)) {
				v.errp("R05", "lead_cannot_work", tp+".role", map[string]any{"role": t.Role}, "the lead plans and does not take tasks")
			}
			v.checkInstancePool(tp, t)
			v.checkDeadlineKind(tp, t)
		}
	}

	for i, r := range d.Roles {
		rp := rolePath(label(r.Key, i))
		builtin := r.Kind == RoleKindLead || r.Kind == RoleKindRequester || r.Kind == RoleKindManagers
		if builtin {
			continue
		}
		// R04
		if !used[r.Key] {
			v.errp("R04", "role_unused", rp, map[string]any{"role": r.Key}, "role %q is used by no task", r.Key)
		}
		v.checkBinding(rp, r)
	}
}

// checkBinding is R02 and R03 for one role.
func (v *validator) checkBinding(rp string, r Role) {
	roster := v.ctx.Roster
	var ids []string
	if r.Bind != nil {
		switch r.Kind {
		case RoleKindAgent:
			ids = r.Bind.AgentIDs
			if len(r.Bind.MemberIDs) > 0 {
				v.errp("R03", "role_not_member", rp+".bind", map[string]any{"role": r.Key}, "an agent role binds agent_ids")
			}
		case RoleKindMember:
			ids = r.Bind.MemberIDs
			if len(r.Bind.AgentIDs) > 0 {
				v.errp("R03", "role_not_member", rp+".bind", map[string]any{"role": r.Key}, "a member role binds member_ids")
			}
		}
	}
	need := max(r.Pool, 1)
	if v.ctx.Publish && len(ids) < need {
		v.errp("R02", "role_unbound", rp+".bind", map[string]any{"role": r.Key, "need": need, "bound": len(ids)}, "role %q needs %d bound, has %d", r.Key, need, len(ids))
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			v.errp("R03", "role_not_member", rp+".bind", map[string]any{"role": r.Key, "id": id}, "%q is bound twice", id)
		}
		seen[id] = true
		switch r.Kind {
		case RoleKindAgent:
			switch {
			case roster.Lead != "" && id == roster.Lead:
				v.errp("R03", "role_is_lead", rp+".bind", map[string]any{"role": r.Key, "agent_id": id}, "the lead of the squad cannot take tasks")
			case !inStrings(roster.Agents, id) || roster.Archived[id]:
				v.errp("R03", "role_not_member", rp+".bind", map[string]any{"role": r.Key, "agent_id": id}, "agent %q is not an active agent member of the squad", id)
			}
		case RoleKindMember:
			if v.ctx.WorkspaceMembers != nil && !v.ctx.WorkspaceMembers[id] {
				v.errp("R03", "role_not_member", rp+".bind", map[string]any{"role": r.Key, "member_id": id}, "member %q is not a member of the workspace", id)
			}
		}
	}
}

// checkInstancePool is R06.
func (v *validator) checkInstancePool(tp string, t DocTask) {
	in := t.Instances
	if in == nil {
		return
	}
	need := in.Count
	if in.FromVar != "" {
		need = in.Max
		if need < 1 {
			need = DocMaxInstances
		}
		if x, ok := v.vars[in.FromVar]; !ok || x.Type != VarTypeListString {
			v.errp("R06", "instance_var_invalid", tp+".instances.from_var", map[string]any{"var": in.FromVar}, "instances come from %q, which is not a list_string variable", in.FromVar)
		}
		if in.Count > 0 {
			v.err("R06", "instance_var_invalid", tp+".instances", "instances has a count or a from_var, not both")
		}
	} else if in.Count < 1 {
		v.err("R06", "instance_var_invalid", tp+".instances", "instances needs a count or a from_var")
	}
	if in.FromVar != "" && in.Min > 0 && in.Max > 0 && in.Min > in.Max {
		v.err("R06", "instance_var_invalid", tp+".instances", "instances: min is more than max")
	}
	if pool := len(v.people(t.Role)); in.IsDistinct() && pool < need {
		v.errp("R06", "pool_too_small", tp+".instances", map[string]any{"role": t.Role, "pool": pool, "need": need}, "%d distinct instances need a pool of %d; role %q has %d", need, need, t.Role, pool)
	}
}

// checkDeadlineKind is R08: an agent deadline for an agent role and a person's
// for a human one.
func (v *validator) checkDeadlineKind(tp string, t DocTask) {
	dl := t.Deadline
	if dl == nil {
		return
	}
	human := isHumanKind(v.roleKind(t.Role))
	agentSide := dl.IdleMinutes != 0 || dl.HardCapMinutes != 0 || dl.MaxAttempts != 0
	humanSide := dl.RemindAfterMinutes != 0 || dl.StallAfterMinutes != 0
	if human && agentSide {
		v.errp("R08", "deadline_kind_invalid", tp+".deadline", map[string]any{"role": t.Role}, "role %q is a person: use remind_after_minutes and stall_after_minutes", t.Role)
	}
	if !human && humanSide {
		v.errp("R08", "deadline_kind_invalid", tp+".deadline", map[string]any{"role": t.Role}, "role %q is an agent: use idle_minutes, hard_cap_minutes and max_attempts", t.Role)
	}
}

// predecessors are the steps with an edge into each step.
func (v *validator) predecessors() map[string][]string {
	out := map[string][]string{}
	for from, tos := range v.succ {
		for _, to := range tos {
			out[to] = append(out[to], from)
		}
	}
	for k := range out {
		sort.Strings(out[k])
	}
	return out
}

type stepTask struct {
	step string
	task DocTask
}

// judgedWork are the work tasks a gate judges (spec 4.11, R07): the work tasks
// among its declared inputs; without inputs, the work tasks of the nearest steps
// before it on any path, and the work tasks of its own step that it comes after.
func (v *validator) judgedWork(stepKey string, gate DocTask) []stepTask {
	var out []stepTask
	add := func(step string, t DocTask) {
		if t.Kind == KindWork {
			out = append(out, stepTask{step, t})
		}
	}
	if len(gate.Inputs) > 0 {
		for _, in := range gate.Inputs {
			step, key, ok := strings.Cut(in.From, ".")
			if t, found := v.tasks[step+"."+key]; ok && found {
				add(step, t)
			}
		}
		return out
	}
	preds := v.predecessors()
	seen := map[string]bool{stepKey: true}
	queue := append([]string(nil), preds[stepKey]...)
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if seen[cur] {
			continue
		}
		seen[cur] = true
		s := v.steps[cur]
		hasWork := false
		for _, t := range s.Tasks {
			if t.Kind == KindWork {
				hasWork = true
				add(cur, t)
			}
		}
		if !hasWork {
			queue = append(queue, preds[cur]...)
		}
	}
	for _, dep := range v.afterClosure(stepKey, gate) {
		add(stepKey, dep)
	}
	return out
}

// afterClosure are the tasks of the step that the task comes after, directly or not.
func (v *validator) afterClosure(stepKey string, t DocTask) []DocTask {
	var out []DocTask
	seen := map[string]bool{}
	var walk func(DocTask)
	walk = func(cur DocTask) {
		for _, dep := range cur.After {
			if seen[dep] {
				continue
			}
			seen[dep] = true
			if d, ok := v.tasks[stepKey+"."+dep]; ok {
				out = append(out, d)
				walk(d)
			}
		}
	}
	walk(t)
	return out
}

// checkSeparation is R07. A gate must not judge only the work of the people who
// fill it. Every person of the gate's role who could have done the judged work
// is taken away, as many as the work has instances; the gate must still have the
// people it needs. A role that shares no one with the work leaves the gate whole.
func (v *validator) checkSeparation() {
	for i, s := range v.doc.Steps {
		sp := label(s.Key, i)
		gateRoles := map[string][]DocTask{}
		for j, t := range s.Tasks {
			if t.Kind != KindGate {
				continue
			}
			tp := taskPath(sp, label(t.Key, j))
			gateRoles[t.Role] = append(gateRoles[t.Role], t)
			judged := v.judgedWork(s.Key, t)
			if len(judged) == 0 {
				continue
			}
			pool := v.people(t.Role)
			inPool := map[string]bool{}
			for _, p := range pool {
				inPool[p] = true
			}
			overlap := map[string]bool{}
			uses := 0
			var names []string
			for _, w := range judged {
				shared := false
				for _, p := range v.people(w.task.Role) {
					if inPool[p] {
						overlap[p] = true
						shared = true
					}
				}
				if shared {
					uses += maxInstances(w.task)
					names = append(names, w.step+"."+w.task.Key)
				}
			}
			if len(overlap) == 0 {
				continue
			}
			need := 1
			if t.Instances != nil && t.Instances.IsDistinct() {
				need = maxInstances(t)
			}
			free := len(pool) - min(uses, len(overlap))
			if free < need {
				v.errp("R07", "separation_violation", tp+".role", map[string]any{"gate": t.Key, "role": t.Role, "judged": names, "free": free, "need": need},
					"gate %q cannot be given to anyone who did the work it judges (%s)", t.Key, strings.Join(names, ", "))
			}
		}
		// Gates of one step are independent of each other.
		roleKeys := make([]string, 0, len(gateRoles))
		for role := range gateRoles {
			roleKeys = append(roleKeys, role)
		}
		sort.Strings(roleKeys)
		for _, role := range roleKeys {
			gates := gateRoles[role]
			if len(gates) < 2 {
				continue
			}
			// Name the gates by key order, so the issue does not move with the document.
			sort.Slice(gates, func(a, b int) bool { return gates[a].Key < gates[b].Key })
			need := 0
			for _, g := range gates {
				if g.Instances != nil && g.Instances.IsDistinct() {
					need += maxInstances(g)
				} else {
					need++
				}
			}
			if pool := len(v.people(role)); pool < need {
				v.errp("R07", "separation_violation", stepPath(sp)+".tasks["+gates[1].Key+"].role", map[string]any{"gate": gates[1].Key, "role": role, "free": pool, "need": need, "reason": "gates_share_role"},
					"gates %q and %q of one step need %d different people; role %q has %d", gates[0].Key, gates[1].Key, need, role, pool)
			}
		}
	}
}

// checkSkills is R09: a skill a task names is on every agent of its role.
func (v *validator) checkSkills() {
	if !v.ctx.Publish {
		return
	}
	for i, s := range v.doc.Steps {
		for j, t := range s.Tasks {
			if len(t.Skills) == 0 {
				continue
			}
			tp := taskPath(label(s.Key, i), label(t.Key, j))
			r, ok := v.roles[t.Role]
			if !ok {
				continue
			}
			if isHumanKind(r.Kind) {
				v.errp("R09", "skill_not_attached", tp+".skills", map[string]any{"role": t.Role, "reason": "human_role"}, "role %q is a person and has no skills", t.Role)
				continue
			}
			if r.Bind == nil {
				continue
			}
			for _, id := range r.Bind.AgentIDs {
				for _, skill := range t.Skills {
					if !inStrings(v.ctx.Roster.AgentSkills[id], skill) {
						v.errp("R09", "skill_not_attached", tp+".skills", map[string]any{"role": t.Role, "agent_id": id, "skill": skill}, "agent %q does not have the skill %q", id, skill)
					}
				}
			}
		}
	}
}

// ---------------------------------------------------------------- T: tasks

func (v *validator) checkTasks() {
	v.checkSeparation()
	v.checkSkills()
	for i, set := range v.doc.VerdictSets {
		v.checkVerdictSet(setPath(label(set.Key, i)), set)
	}
	for i, s := range v.doc.Steps {
		sk := label(s.Key, i)
		for j, t := range s.Tasks {
			tp := taskPath(sk, label(t.Key, j))
			v.checkTaskBasics(tp, s, t)
			v.checkFiles(tp, t)
			v.checkDeadline(tp, t)
		}
		v.checkAfter(sk, s)
		v.checkAvailability(sk, s)
		v.checkInputs(sk, s)
		v.checkSets(sk, s)
	}
	v.checkMaybeUnset()
}

// checkVerdictSet is the part of T01 that concerns a declared set.
func (v *validator) checkVerdictSet(sp string, set VerdictSet) {
	passes := 0
	for _, vd := range set.Verdicts {
		switch vd.Class {
		case ClassPass:
			passes++
		case ClassFail, ClassNeutral:
		default:
			v.errp("T01", "verdict_set_invalid", sp, map[string]any{"set": set.Key, "verdict": vd.Key, "class": vd.Class}, "verdict %q has the class %q; the classes are pass, fail and neutral", vd.Key, vd.Class)
		}
		if vd.Key == OutcomeBlocked {
			v.errp("T01", "verdict_set_invalid", sp, map[string]any{"set": set.Key, "verdict": vd.Key}, "blocked is always valid and is not listed in a set")
		}
	}
	if len(set.Verdicts) == 0 || passes == 0 {
		v.errp("T01", "verdict_set_invalid", sp, map[string]any{"set": set.Key}, "set %q needs at least one verdict and one with the class pass", set.Key)
	}
}

// checkTaskBasics is T01 for a task and T02.
func (v *validator) checkTaskBasics(tp string, s DocStep, t DocTask) {
	if t.Kind != KindWork && t.Kind != KindGate {
		v.errp("T01", "task_kind_invalid", tp+".kind", map[string]any{"kind": t.Kind}, "task %q: kind must be work or gate", t.Key)
	}
	if t.Verdicts != "" && !v.setKnown(t.Verdicts) {
		v.errp("T01", "verdict_set_invalid", tp+".verdicts", map[string]any{"set": t.Verdicts}, "task %q: the verdict set %q is not declared", t.Key, t.Verdicts)
		return
	}
	if t.Kind == KindGate {
		classes := map[string]bool{}
		for _, vd := range v.taskVerdicts(t) {
			classes[vd.Class] = true
		}
		if !classes[ClassPass] || !classes[ClassFail] {
			v.errp("T01", "verdict_set_invalid", tp+".verdicts", map[string]any{"task": t.Key}, "gate %q needs a verdict set with a pass and a fail", t.Key)
		}
		if t.Requires != nil && t.Requires.Verdict != nil && !*t.Requires.Verdict {
			v.errp("T02", "gate_verdict_required", tp+".requires.verdict", map[string]any{"task": t.Key}, "gate %q must require a verdict", t.Key)
		}
	}
}

// checkFiles is T05.
func (v *validator) checkFiles(tp string, t DocTask) {
	if t.Requires == nil {
		return
	}
	slots := map[string]bool{}
	for k, f := range t.Requires.Files {
		fp := tp + ".requires.files[" + label(f.Slot, k) + "]"
		switch {
		case f.Slot == "":
			v.err("T05", "file_req_invalid", fp, "a file requirement needs a slot")
		case slots[f.Slot]:
			v.errp("T05", "file_req_invalid", fp, map[string]any{"slot": f.Slot}, "slot %q is used twice", f.Slot)
		}
		slots[f.Slot] = true
		if f.Min < 1 || f.Min > docMaxFileMin {
			v.errp("T05", "file_req_invalid", fp+".min", map[string]any{"slot": f.Slot}, "min is 1 to %d", docMaxFileMin)
		}
		if f.Max != 0 && f.Max < f.Min {
			v.errp("T05", "file_req_invalid", fp+".max", map[string]any{"slot": f.Slot}, "max is not less than min")
		}
		if len(f.Ext) > docMaxExtensions {
			v.errp("T05", "file_req_invalid", fp+".ext", map[string]any{"slot": f.Slot}, "at most %d extensions", docMaxExtensions)
		}
		for _, e := range f.Ext {
			if !extPattern.MatchString(e) {
				v.errp("T05", "file_req_invalid", fp+".ext", map[string]any{"slot": f.Slot, "ext": e}, "extension %q must be lower case and start with a dot", e)
			}
		}
	}
}

// checkDeadline is T06.
func (v *validator) checkDeadline(tp string, t DocTask) {
	dl := t.Deadline
	if dl == nil {
		return
	}
	bad := func(field string, n, max int) {
		if n < 0 || n > max {
			v.errp("T06", "deadline_invalid", tp+".deadline."+field, map[string]any{"field": field, "max": max, "actual": n}, "%s is 0 to %d", field, max)
		}
	}
	bad("idle_minutes", dl.IdleMinutes, docMaxDeadlineMinutes)
	bad("hard_cap_minutes", dl.HardCapMinutes, docMaxDeadlineMinutes)
	bad("max_attempts", dl.MaxAttempts, docMaxDeadlineAttempts)
	bad("remind_after_minutes", dl.RemindAfterMinutes, docMaxDeadlineMinutes)
	bad("stall_after_minutes", dl.StallAfterMinutes, docMaxDeadlineMinutes)
	if dl.IdleMinutes > 0 && dl.HardCapMinutes > 0 && dl.IdleMinutes > dl.HardCapMinutes {
		v.errp("T06", "deadline_invalid", tp+".deadline", nil, "idle_minutes is more than hard_cap_minutes")
	}
	if dl.RemindAfterMinutes > 0 && dl.StallAfterMinutes > 0 && dl.RemindAfterMinutes >= dl.StallAfterMinutes {
		v.errp("T06", "deadline_invalid", tp+".deadline", nil, "remind_after_minutes must be before stall_after_minutes")
	}
}

// checkAfter is T03.
func (v *validator) checkAfter(sk string, s DocStep) {
	byKey := map[string]DocTask{}
	for _, t := range s.Tasks {
		byKey[t.Key] = t
	}
	depth := map[string]int{}
	state := map[string]int{}
	var visit func(key string) int
	visit = func(key string) int {
		if state[key] == 2 {
			return depth[key]
		}
		if state[key] == 1 {
			return -1
		}
		state[key] = 1
		longest := 0
		for _, dep := range byKey[key].After {
			if _, ok := byKey[dep]; !ok {
				continue
			}
			d := visit(dep)
			if d < 0 {
				return -1
			}
			longest = max(longest, d+1)
		}
		state[key] = 2
		depth[key] = longest
		return longest
	}
	for j, t := range s.Tasks {
		tp := taskPath(sk, label(t.Key, j))
		for _, dep := range t.After {
			dt, ok := byKey[dep]
			switch {
			case dep == t.Key:
				v.errp("T03", "after_invalid", tp+".after", map[string]any{"task": t.Key}, "task %q comes after itself", t.Key)
			case !ok:
				v.errp("T03", "after_invalid", tp+".after", map[string]any{"task": t.Key, "after": dep}, "task %q comes after %q, which is not a task of this step", t.Key, dep)
			case dt.When != nil:
				v.warn("T03", "after_maybe_skipped", tp+".after", map[string]any{"task": t.Key, "after": dep},
					"task %q comes after %q, which may not open; a task that did not open does not hold it back", t.Key, dep)
			}
		}
		if d := visit(t.Key); d < 0 {
			v.errp("T03", "after_invalid", tp+".after", map[string]any{"task": t.Key}, "the after chain of %q is a cycle", t.Key)
			for k := range state {
				state[k] = 2
			}
		} else if d > DocMaxAfterDepth {
			v.errp("T03", "after_invalid", tp+".after", map[string]any{"task": t.Key, "depth": d, "max": DocMaxAfterDepth}, "the after chain of %q is %d deep; the limit is %d", t.Key, d, DocMaxAfterDepth)
		}
	}
}

// checkAvailability is T04: when and instances use only facts that exist when the
// step is entered (spec 2.6 rule 3).
func (v *validator) checkAvailability(sk string, s DocStep) {
	setters := map[string][]string{} // variable -> tasks of this step that set it
	for _, t := range s.Tasks {
		for _, name := range t.Sets {
			setters[name] = append(setters[name], t.Key)
		}
	}
	for j, t := range s.Tasks {
		tp := taskPath(sk, label(t.Key, j))
		after := map[string]bool{}
		for _, dep := range v.afterClosure(s.Key, t) {
			after[dep.Key] = true
		}
		var fields []string
		if t.When != nil {
			walkCondition(*t.When, func(c Condition) { fields = append(fields, c.Field) })
		}
		if t.Instances != nil && t.Instances.FromVar != "" {
			fields = append(fields, "var."+t.Instances.FromVar)
		}
		for _, f := range fields {
			parts := strings.Split(f, ".")
			switch {
			case parts[0] == "join" || f == "lead.verdict":
				v.errp("T04", "fact_not_available", tp+".when", map[string]any{"task": t.Key, "fact": f}, "%s does not exist yet when step %q is entered", f, s.Key)
			case parts[0] == "task" && len(parts) == 3:
				if _, same := v.tasks[s.Key+"."+parts[1]]; same {
					v.errp("T04", "fact_not_available", tp+".when", map[string]any{"task": t.Key, "fact": f}, "%s is a task of the same step, which does not exist yet when it is entered", f)
				}
			case parts[0] == "var" && len(parts) == 2:
				for _, setter := range setters[parts[1]] {
					if setter != t.Key && !after[setter] {
						v.errp("T04", "fact_not_available", tp+".when", map[string]any{"task": t.Key, "fact": f, "setter": setter}, "%s is set by %q in this step; %q must come after it", f, setter, t.Key)
					}
				}
			}
		}
	}
}

// checkInputs is T07.
func (v *validator) checkInputs(sk string, s DocStep) {
	forward := reachableByStep(v.fwd)
	for j, t := range s.Tasks {
		tp := taskPath(sk, label(t.Key, j))
		for k, in := range t.Inputs {
			ip := fmt.Sprintf("%s.inputs[%s]", tp, label(in.From, k))
			stepKey, taskKey, ok := strings.Cut(in.From, ".")
			src, found := v.tasks[stepKey+"."+taskKey]
			switch {
			case !ok || !found:
				v.errp("T07", "input_invalid", ip, map[string]any{"from": in.From}, "input %q is not a task of the flow (use step.task)", in.From)
				continue
			case stepKey == s.Key:
				v.errp("T07", "input_invalid", ip, map[string]any{"from": in.From}, "input %q is in this step: use after for tasks of the same step", in.From)
				continue
			case !forward(stepKey)[s.Key]:
				v.errp("T07", "input_invalid", ip, map[string]any{"from": in.From}, "step %q does not lead to step %q", stepKey, s.Key)
				continue
			}
			if in.Slot != "" {
				found := false
				if src.Requires != nil {
					for _, f := range src.Requires.Files {
						found = found || f.Slot == in.Slot
					}
				}
				if !found {
					v.errp("T07", "input_invalid", ip, map[string]any{"from": in.From, "slot": in.Slot}, "task %q declares no file slot %q", in.From, in.Slot)
				}
			}
			if !v.dominates(stepKey, s.Key) || src.When != nil {
				sev := SeverityWarning
				if in.Required {
					sev = SeverityError
				}
				v.add("T07", "input_maybe_missing", sev, ip, fmt.Sprintf("input %q may not exist when step %q is entered", in.From, s.Key), map[string]any{"from": in.From, "required": in.Required})
			}
			if src.Requires == nil || len(src.Requires.Files) == 0 {
				// The task reads the files attached to its input; a source that requires none has none to read.
				v.add("T07", "input_no_files", SeverityWarning, ip, fmt.Sprintf("input %q requires no file, so there is no attached file to read", in.From), map[string]any{"from": in.From})
			}
		}
	}
}

// reachableByStep returns a memoised reachability over the successors.
func reachableByStep(next map[string][]string) func(string) map[string]bool {
	cache := map[string]map[string]bool{}
	return func(from string) map[string]bool {
		if r, ok := cache[from]; ok {
			return r
		}
		r := reachableFrom(from, next, "")
		delete(r, from)
		// A step reaches itself only through a cycle, which forward edges do not have.
		cache[from] = r
		return r
	}
}

// checkSets is T08 for the variables a task sets.
func (v *validator) checkSets(sk string, s DocStep) {
	for j, t := range s.Tasks {
		tp := taskPath(sk, label(t.Key, j))
		for _, name := range t.Sets {
			x, ok := v.vars[name]
			switch {
			case !ok:
				v.errp("T08", "var_set_invalid", tp+".sets", map[string]any{"task": t.Key, "var": name}, "task %q sets %q, which is not a variable", t.Key, name)
			case !inStrings(x.SetBy, t.Role):
				v.errp("T08", "var_set_invalid", tp+".sets", map[string]any{"task": t.Key, "var": name, "role": t.Role}, "variable %q cannot be set by role %q (set_by)", name, t.Role)
			case !inStrings(x.SetAt, s.Key) && !inStrings(x.SetAt, "*"):
				v.errp("T08", "var_set_invalid", tp+".sets", map[string]any{"task": t.Key, "var": name, "step": s.Key}, "variable %q cannot be set at step %q (set_at)", name, s.Key)
			}
		}
		if t.Requires != nil {
			for _, name := range t.Requires.Vars {
				if !inStrings(t.Sets, name) {
					v.errp("T08", "var_set_invalid", tp+".requires.vars", map[string]any{"task": t.Key, "var": name}, "task %q requires %q but does not set it", t.Key, name)
				}
			}
		}
	}
}

// checkMaybeUnset is the warning of T08: a variable with no default that a step
// reads but that nothing sets before it on every path.
func (v *validator) checkMaybeUnset() {
	setterSteps := map[string][]string{}
	for _, s := range v.doc.Steps {
		if s.Lead != nil {
			for _, name := range s.Lead.Sets {
				setterSteps[name] = append(setterSteps[name], s.Key)
			}
		}
		for _, t := range s.Tasks {
			for _, name := range t.Sets {
				setterSteps[name] = append(setterSteps[name], s.Key)
			}
		}
	}
	for i, s := range v.doc.Steps {
		reads := map[string]string{}
		for j, t := range s.Tasks {
			var fields []string
			if t.When != nil {
				walkCondition(*t.When, func(c Condition) { fields = append(fields, c.Field) })
			}
			if t.Instances != nil && t.Instances.FromVar != "" {
				fields = append(fields, "var."+t.Instances.FromVar)
			}
			for _, f := range fields {
				if name, ok := strings.CutPrefix(f, "var."); ok {
					if _, seen := reads[name]; !seen {
						reads[name] = taskPath(label(s.Key, i), label(t.Key, j))
					}
				}
			}
		}
		names := make([]string, 0, len(reads))
		for name := range reads {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			x, ok := v.vars[name]
			if !ok || x.Default != nil {
				continue
			}
			before := false
			for _, setter := range setterSteps[name] {
				if setter != s.Key && v.dominates(setter, s.Key) {
					before = true
				}
			}
			if !before {
				v.warn("T08", "var_maybe_unset", reads[name], map[string]any{"var": name, "step": s.Key},
					"variable %q has no default and nothing sets it before step %q on every path", name, s.Key)
			}
		}
	}
}

// ---------------------------------------------------------------- J: joins

func (v *validator) checkJoins() {
	for i, s := range v.doc.Steps {
		sk := label(s.Key, i)
		sp := stepPath(sk)
		if s.Kind == StepKindManual {
			v.checkManual(sk, s)
		} else if s.ExitRole != "" {
			v.errp("J03", "manual_step_invalid", sp+".exit_role", map[string]any{"step": s.Key}, "only a manual step has an exit_role")
		}
		if s.Join != nil && s.Kind != StepKindTasks {
			v.errp("J01", "join_invalid", sp+".join", map[string]any{"step": s.Key}, "only a tasks step has a join")
		}
		if s.Kind != StepKindTasks {
			continue
		}
		join := s.EffectiveJoin()
		jp := sp + ".join"
		switch join.Mode {
		case JoinModeAll, JoinModeAny, JoinModeQuorum:
		default:
			v.errp("J01", "join_invalid", jp+".mode", map[string]any{"mode": join.Mode}, "join mode must be all, any or quorum")
		}
		switch join.Block {
		case JoinBlockImmediate, JoinBlockAtClose:
		default:
			v.errp("J01", "join_invalid", jp+".block", map[string]any{"block": join.Block}, "join block must be immediate or at_close")
		}
		if join.Over.Kind == JoinOverTasks {
			if len(join.Over.Tasks) == 0 {
				v.err("J01", "join_invalid", jp+".over", "a join over a list needs at least one task")
			}
			for _, key := range join.Over.Tasks {
				if _, ok := v.tasks[s.Key+"."+key]; !ok {
					v.errp("J01", "join_invalid", jp+".over", map[string]any{"task": key}, "the join is over %q, which is not a task of this step", key)
				}
			}
		}
		if join.Mode == JoinModeQuorum {
			certain := 0
			for _, t := range v.overTasks(s) {
				certain += certainInstances(t)
			}
			if join.K < 1 || join.K > certain {
				v.errp("J01", "join_invalid", jp+".k", map[string]any{"k": join.K, "certain": certain}, "quorum k=%d needs 1 to %d instances that are certain to open", join.K, certain)
			}
		}
		// J02
		if missing := resultsCovered(s.Edges, "join.result", v.joinDomain(s)); len(missing) > 0 {
			v.errp("J02", "join_result_unrouted", sp, map[string]any{"step": s.Key, "unrouted": missing}, "no edge of step %q takes the result %s", s.Key, strings.Join(missing, ", "))
		}
	}
}

// certainInstances counts the rows of a task that are sure to open: none when it
// has a condition, otherwise its count, or the least a list gives.
func certainInstances(t DocTask) int {
	switch {
	case t.When != nil:
		return 0
	case t.Instances == nil:
		return 1
	case t.Instances.FromVar != "":
		return max(t.Instances.Min, 1)
	}
	return t.Instances.Count
}

// checkManual is J03.
func (v *validator) checkManual(sk string, s DocStep) {
	sp := stepPath(sk)
	labels := map[string]bool{}
	for j, e := range s.Edges {
		ep := edgePath(sk, label(e.ID, j))
		if e.Trigger != EdgeTriggerManual {
			v.errp("J03", "manual_step_invalid", ep+".trigger", map[string]any{"edge": e.ID}, "every edge of a manual step is a button a person presses")
		}
		key := strings.ToLower(strings.TrimSpace(e.Label))
		if labels[key] {
			v.errp("J03", "manual_step_invalid", ep+".label", map[string]any{"edge": e.ID}, "two buttons of the step are called %q", e.Label)
		}
		labels[key] = true
	}
	if s.ExitRole != "" && !isHumanKind(v.roleKind(s.ExitRole)) {
		v.errp("J03", "manual_step_invalid", sp+".exit_role", map[string]any{"role": s.ExitRole}, "the exit role %q must be a member, requester or managers role", s.ExitRole)
	}
}

// ---------------------------------------------------------------- F: facts and variables

type condSite struct {
	path string
	cond Condition
	// step is the step whose edge or task holds the condition; edge says which.
	step DocStep
	edge bool
}

func (v *validator) conditionSites() []condSite {
	var out []condSite
	for i, s := range v.doc.Steps {
		sk := label(s.Key, i)
		for j, t := range s.Tasks {
			if t.When != nil {
				out = append(out, condSite{taskPath(sk, label(t.Key, j)) + ".when", *t.When, s, false})
			}
		}
		for j, e := range s.Edges {
			if e.When != nil {
				out = append(out, condSite{edgePath(sk, label(e.ID, j)) + ".when", *e.When, s, true})
			}
		}
	}
	return out
}

func (v *validator) checkFacts() {
	used := map[string]bool{}
	for _, site := range v.conditionSites() {
		v.checkCondition(site, site.cond, 1, used)
	}
	for _, s := range v.doc.Steps {
		for _, t := range s.Tasks {
			if t.Instances != nil && t.Instances.FromVar != "" {
				used[t.Instances.FromVar] = true
			}
		}
	}
	for _, l := range v.doc.Loops {
		if l.Max.FromVar() {
			used[l.Max.Var] = true
		}
	}
	v.checkVars(used)
	v.checkLoopMax()
}

// checkCondition is F01, F02 and F06 for one condition tree.
func (v *validator) checkCondition(site condSite, c Condition, depth int, used map[string]bool) {
	forms := 0
	for _, present := range []bool{c.All != nil, c.Any != nil, c.Not != nil, c.Field != "" || c.Op != ""} {
		if present {
			forms++
		}
	}
	if forms != 1 {
		v.err("F02", "condition_invalid", site.path, "a condition is a group (all or any), a negation (not), or a field check")
		return
	}
	switch {
	case c.All != nil:
		if len(c.All) == 0 {
			v.err("F02", "condition_invalid", site.path, "an all group needs at least one condition")
		}
		for _, child := range c.All {
			v.checkCondition(site, child, depth+1, used)
		}
		return
	case c.Any != nil:
		if len(c.Any) == 0 {
			v.err("F02", "condition_invalid", site.path, "an any group needs at least one condition")
		}
		for _, child := range c.Any {
			v.checkCondition(site, child, depth+1, used)
		}
		return
	case c.Not != nil:
		v.checkCondition(site, *c.Not, depth+1, used)
		return
	}

	spec, issue := LookupFact(c.Field, v.doc.Vars)
	if issue != nil {
		rule := "F01"
		if issue.Code == CodeLegacyFieldInV2 {
			rule = "F06"
		}
		v.add(rule, issue.Code, SeverityError, site.path, issue.Message, map[string]any{"field": c.Field})
		return
	}
	parts := strings.Split(c.Field, ".")
	switch {
	case parts[0] == "var":
		used[parts[1]] = true
	case parts[0] == "task" && len(parts) == 3:
		if !v.taskExists(parts[1]) {
			v.errp("F01", CodeUnknownFact, site.path, map[string]any{"field": c.Field}, "%q: the flow has no task %q", c.Field, parts[1])
			return
		}
	case parts[0] == "loop" && len(parts) == 3:
		if _, ok := v.loops[parts[1]]; !ok {
			v.errp("F01", CodeUnknownFact, site.path, map[string]any{"field": c.Field}, "%q: the flow has no loop %q", c.Field, parts[1])
			return
		}
	case parts[0] == "step" && len(parts) == 3:
		if _, ok := v.steps[parts[1]]; !ok {
			v.errp("F01", CodeUnknownFact, site.path, map[string]any{"field": c.Field}, "%q: the flow has no step %q", c.Field, parts[1])
			return
		}
	case parts[0] == "join" && site.edge && site.step.Kind == StepKindLead,
		c.Field == "lead.verdict" && (!site.edge || site.step.Kind != StepKindLead):
		v.errp("F01", "fact_not_available", site.path, map[string]any{"field": c.Field}, "%s is not available here", c.Field)
		return
	}
	if !hasOp(spec.Ops, c.Op) {
		v.errp("F02", "fact_op_invalid", site.path, map[string]any{"field": c.Field, "op": c.Op}, "operator %q does not apply to %s", c.Op, c.Field)
		return
	}
	if c.Op == OpExists {
		if c.Value != nil {
			v.errp("F02", "fact_value_invalid", site.path, map[string]any{"field": c.Field}, "operator exists takes no value")
		}
		return
	}
	if err := checkOperand(spec, c); err != nil {
		v.errp("F02", "fact_value_invalid", site.path, map[string]any{"field": c.Field}, "%s", err.Message)
		return
	}
	if parts[0] == "var" && spec.Type == VarTypeInt {
		x := v.vars[parts[1]]
		operands := []any{c.Value}
		if c.Op == OpIn {
			operands = asList(c.Value)
		}
		for _, operand := range operands {
			n, _ := asNumber(operand)
			if (x.Min != nil && n < float64(*x.Min)) || (x.Max != nil && n > float64(*x.Max)) {
				v.errp("F02", "fact_value_invalid", site.path, map[string]any{"field": c.Field}, "%v is outside the range of %s", operand, c.Field)
			}
		}
	}
}

func (v *validator) taskExists(key string) bool {
	for _, s := range v.doc.Steps {
		for _, t := range s.Tasks {
			if t.Key == key {
				return true
			}
		}
	}
	return false
}

// checkVars is F03 and F04.
func (v *validator) checkVars(used map[string]bool) {
	setBy := func(role string) bool { return v.isBuiltinRole(role) || v.roles[role].Key != "" }
	for i, x := range v.doc.Vars {
		vp := varPath(label(x.Key, i))
		bad := func(format string, args ...any) {
			v.errp("F03", "var_invalid", vp, map[string]any{"var": x.Key}, format, args...)
		}
		switch x.Type {
		case VarTypeBool:
		case VarTypeEnum:
			if len(x.Values) == 0 || len(x.Values) > docMaxEnumValues {
				bad("an enum has 1 to %d values", docMaxEnumValues)
			}
			seen := map[string]bool{}
			for _, val := range x.Values {
				if val == "" || seen[val] {
					bad("enum values are non-empty and distinct")
				}
				seen[val] = true
			}
		case VarTypeInt:
			if x.Min != nil && x.Max != nil && *x.Min > *x.Max {
				bad("min is more than max")
			}
		case VarTypeString:
			if x.MaxLen < 0 || x.MaxLen > docMaxVarStringRunes {
				bad("max_len is 1 to %d", docMaxVarStringRunes)
			}
		case VarTypeListString:
			if x.MaxItems < 0 || x.MaxItems > docMaxVarListItems {
				bad("max_items is 1 to %d", docMaxVarListItems)
			}
		default:
			bad("type must be bool, enum, int, string or list_string")
			continue
		}
		v.checkVarDefault(vp, x)
		for _, role := range x.SetBy {
			if !setBy(role) {
				v.errp("F03", "var_invalid", vp+".set_by", map[string]any{"var": x.Key, "role": role}, "set_by names %q, which is not a role", role)
			} else if r, declared := v.roles[role]; declared && !isHumanKind(r.Kind) && r.Kind != RoleKindLead && !v.rolesSetting(role, x.Key) {
				v.errp("F03", "var_invalid", vp+".set_by", map[string]any{"var": x.Key, "role": role}, "role %q is in set_by but no task of that role sets %q", role, x.Key)
			}
		}
		for _, step := range x.SetAt {
			if _, ok := v.steps[step]; step != "*" && !ok {
				v.errp("F03", "var_invalid", vp+".set_at", map[string]any{"var": x.Key, "step": step}, "set_at names %q, which is not a step", step)
			}
		}
		if !used[x.Key] {
			v.warn("F04", "var_unused", vp, map[string]any{"var": x.Key}, "variable %q is read by no condition, instance list or loop", x.Key)
		}
	}
}

// rolesSetting reports whether a task of the role lists the variable in sets.
func (v *validator) rolesSetting(role, name string) bool {
	for _, s := range v.doc.Steps {
		for _, t := range s.Tasks {
			if t.Role == role && inStrings(t.Sets, name) {
				return true
			}
		}
	}
	return false
}

// checkVarDefault checks the default of a variable against its domain.
func (v *validator) checkVarDefault(vp string, x Var) {
	if x.Default == nil {
		return
	}
	ok := true
	switch x.Type {
	case VarTypeBool:
		_, ok = x.Default.(bool)
	case VarTypeEnum:
		s, isStr := x.Default.(string)
		ok = isStr && inStrings(x.Values, s)
	case VarTypeInt:
		n, isNum := asNumber(x.Default)
		ok = isNum && n == float64(int(n)) && (x.Min == nil || n >= float64(*x.Min)) && (x.Max == nil || n <= float64(*x.Max))
	case VarTypeString:
		s, isStr := x.Default.(string)
		ok = isStr && (x.MaxLen == 0 || runes(s) <= x.MaxLen) && runes(s) <= docMaxVarStringRunes
	case VarTypeListString:
		var list []string
		list, ok = x.Default.([]string)
		if items, isAny := x.Default.([]any); isAny {
			ok = true
			for _, item := range items {
				s, isStr := item.(string)
				ok = ok && isStr
				list = append(list, s)
			}
		}
		ok = ok && (x.MaxItems == 0 || len(list) <= x.MaxItems) && len(list) <= docMaxVarListItems
	}
	if !ok {
		v.errp("F03", "var_invalid", vp+".default", map[string]any{"var": x.Key}, "the default of %q is outside its type", x.Key)
	}
}

// checkLoopMax is F05.
func (v *validator) checkLoopMax() {
	for i, l := range v.doc.Loops {
		lp := loopPath(label(l.Key, i)) + ".max"
		if !l.Max.FromVar() {
			continue
		}
		x, ok := v.vars[l.Max.Var]
		switch {
		case !ok || x.Type != VarTypeInt:
			v.errp("F05", "loop_max_invalid", lp, map[string]any{"loop": l.Key, "var": l.Max.Var}, "loop %q reads its bound from %q, which is not an int variable", l.Key, l.Max.Var)
		case l.Max.Cap < 1 || l.Max.Cap > DocMaxLoopMax:
			v.errp("F05", "loop_max_invalid", lp, map[string]any{"loop": l.Key, "cap": l.Max.Cap}, "the cap of loop %q is 1 to %d", l.Key, DocMaxLoopMax)
		case x.Max == nil || *x.Max > l.Max.Cap:
			v.errp("F05", "loop_max_invalid", lp, map[string]any{"loop": l.Key, "var": l.Max.Var, "cap": l.Max.Cap}, "variable %q must have a max that is at most the cap %d", l.Max.Var, l.Max.Cap)
		}
	}
}

// ---------------------------------------------------------------- P: messages; Y: policy and release

func (v *validator) checkMessagesAndPolicy() {
	d := v.doc
	ref := func(path, key string) {
		if key == "" {
			return
		}
		if _, ok := d.Messages[key]; !ok {
			v.errp("P01", "message_missing", path, map[string]any{"message": key}, "the message %q is not in messages", key)
		}
	}
	for i, s := range d.Stops {
		ref(stopPath(label(s.Code, i))+".message", s.Message)
	}
	for i, s := range d.Steps {
		ref(stepPath(label(s.Key, i))+".notice", s.Notice)
	}
	names := make([]string, 0, len(d.Messages))
	for k := range d.Messages {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		text := d.Messages[k]
		for _, m := range placeholderPattern.FindAllStringSubmatch(text, -1) {
			if !messagePlaceholders[m[1]] {
				v.warn("P01", "message_placeholder", messagePath(k), map[string]any{"placeholder": m[1]}, "{%s} is not a placeholder; the ones that work are %s", m[1], messagePlaceholderList)
			}
		}
		if strings.Contains(strings.ToLower(text), "mention://") {
			v.errp("P02", "message_mention", messagePath(k), map[string]any{"message": k}, "a system message cannot notify a person with a mention link")
		}
	}

	// Y04
	if d.Policy.LeadMayWork && !v.acked(AckLeadMayWork) {
		v.errp("Y04", "ack_required", "policy.lead_may_work", map[string]any{"ack": AckLeadMayWork}, "lead_may_work needs an admin acknowledgement")
	}
	if d.Policy.AllowUnattendedRelease && !v.acked(AckUnattendedRelease) {
		v.errp("Y04", "ack_required", "policy.allow_unattended_release", map[string]any{"ack": AckUnattendedRelease}, "allow_unattended_release needs an admin acknowledgement")
	}

	for i, s := range d.Steps {
		// Y05
		if s.Kind == StepKindManual && s.Notice == "" {
			v.warn("Y05", "manual_without_notice", stepPath(label(s.Key, i)), map[string]any{"step": s.Key}, "manual step %q posts no notice when a request enters it", s.Key)
		}
	}
}

// checkRelease is Y01 to Y03.
func (v *validator) checkRelease() {
	d := v.doc

	// Y02
	gates := 0
	for _, s := range d.Steps {
		for _, t := range s.Tasks {
			if t.Kind == KindGate {
				gates++
			}
		}
	}
	if gates == 0 {
		w := v.warn("Y02", "no_gate", "steps", nil, "no task is a gate: nothing is checked independently")
		if !v.acked(AckNoGate) {
			w.Ack = AckNoGate
		}
	}

	// Y01
	for _, p := range v.unattendedReleases() {
		w := v.warn("Y01", "release_without_human", stepPath(p.From), map[string]any{"step": p.From, "terminal": p.Terminal, "path": p.Path},
			"work of an agent at step %q can leave the workspace by %s without a person looking at it", p.From, strings.Join(p.Path, " > "))
		if !(d.Policy.AllowUnattendedRelease && v.acked(AckUnattendedRelease)) {
			w.Ack = AckUnattendedRelease
		} else {
			w.Params["acknowledged"] = true
		}
	}

	// Y03
	bodies := map[string]map[string]bool{}
	for _, l := range d.Loops {
		body := map[string]bool{}
		if l.Header == "" {
			for _, s := range d.Steps {
				body[s.Key] = true
			}
		}
		for _, key := range v.g.LoopBody(l.Key) {
			body[key] = true
		}
		bodies[l.Key] = body
	}
	for i, a := range d.Loops {
		for _, b := range d.Loops[i+1:] {
			if a.EffectiveReset() != LoopResetNever && b.EffectiveReset() != LoopResetNever {
				continue
			}
			for step := range bodies[a.Key] {
				if bodies[b.Key][step] {
					v.warn("Y03", "loops_overlap", loopPath(b.Key), map[string]any{"loops": []string{a.Key, b.Key}},
						"loops %q and %q share steps and one of them is never reset", a.Key, b.Key)
					break
				}
			}
		}
	}
}

// unattendedReleases are the ways work of an agent leaves the workspace unseen:
// from a step that has an agent work task, a terminal that leaves the workspace
// is reachable without crossing a checkpoint edge (spec 4.8, Y01).
func (v *validator) unattendedReleases() []ReleasePath {
	if _, ok := v.steps[v.doc.Start]; !ok {
		return nil
	}
	reach := reachableFrom(v.doc.Start, v.succ, "")
	var out []ReleasePath
	for _, s := range v.doc.Steps {
		if !reach[s.Key] {
			continue
		}
		agentWork := false
		for _, t := range s.Tasks {
			agentWork = agentWork || (t.Kind == KindWork && v.roleKind(t.Role) == RoleKindAgent)
		}
		if !agentWork {
			continue
		}
		prev := map[string]string{s.Key: ""}
		queue := []string{s.Key}
		var found string
		for len(queue) > 0 && found == "" {
			cur := queue[0]
			queue = queue[1:]
			step := v.steps[cur]
			for _, e := range step.Edges {
				if e.To == "" || isStaticCheckpoint(v.g, step, e) {
					continue
				}
				if _, seen := prev[e.To]; seen {
					continue
				}
				prev[e.To] = cur
				if t, isTerminal := v.terminals[e.To]; isTerminal {
					if t.LeavesWorkspace && found == "" {
						found = e.To
					}
					continue
				}
				queue = append(queue, e.To)
			}
		}
		if found != "" {
			var path []string
			for at := found; at != ""; at = prev[at] {
				path = append([]string{at}, path...)
			}
			out = append(out, ReleasePath{From: s.Key, Terminal: found, Path: path})
		}
	}
	return out
}

// ---------------------------------------------------------------- schema 1

// ValidateSchema1ForSquad checks the role rules of spec section 3.3 on a schema 1
// definition, which ValidateFlow already checked when it was written: ValidateFlow
// has no idea who is in the squad. The definition is read as the document Lift
// makes of it (a fork, without the exemptions of a running graph), and only the
// R rules, the lead rule and the rule that every squad agent is used run on it.
// A definition Lift cannot read gives one error.
func ValidateSchema1ForSquad(def Definition, roster Roster) []Issue {
	g, err := Lift(def, LiftOpts{RoleKeys: roster.RoleKeys})
	if err != nil {
		return []Issue{{Code: "schema1_not_liftable", Severity: SeverityError, Message: err.Error(), Params: map[string]any{ackParamRule: "R01"}}}
	}
	v := newValidator(g.Doc, ValidateContext{Roster: roster})
	v.checkRoles()
	v.checkSeparation()

	// R04 for schema 1: a squad agent that no task uses.
	used := map[string]bool{}
	for _, r := range g.Doc.Roles {
		if r.Bind != nil {
			for _, id := range r.Bind.AgentIDs {
				used[id] = true
			}
		}
	}
	for _, id := range roster.Agents {
		if !used[id] {
			key := roster.RoleKeys[id]
			if key == "" {
				key = id
			}
			v.errp("R04", "role_unused", rolePath(key), map[string]any{"agent_id": id}, "agent %q is a member of the squad and no task uses it", key)
		}
	}
	return v.issues
}
