package dispatch

// SquadLeaderSelfWakeSuppressed reports whether the assigned squad's leader must
// not be enqueued again by its own comment on the assigned issue. The caller
// reads the three facts from the request and the leader's latest task on it.
//
//   - atHumanStop: the request is blocked or in_review, so a person holds it and
//     the leader's own comment is a stop or a handoff.
//   - latestFound: the leader has a task on this issue. Without one, the comment
//     cannot be shown to be a worker report, so the leader is not woken for it.
//   - latestIsSameSquadWorker: the latest task is this squad's worker task, not
//     its leader task. A dual-role leader's worker result still wakes the leader
//     while the request is in progress.
func SquadLeaderSelfWakeSuppressed(atHumanStop, latestFound, latestIsSameSquadWorker bool) bool {
	if atHumanStop || !latestFound {
		return true
	}
	return !latestIsSameSquadWorker
}
