package handler

// flowTaskCreateFork holds fork-specific fields of a flow task being opened.
type flowTaskCreateFork struct {
	Effects string
}

// An external flow task is held by flowTaskMayOpen before the engine creates it (durable spec
// 5.4): the task is opened only once its intent is approved, so the engine needs no hold here.
