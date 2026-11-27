package service

// IssueCreateOptsFork holds fork-specific options for IssueService.Create.
type IssueCreateOptsFork struct {
	// SkipAutoEnqueue creates the issue without enqueueing an automatic agent or
	// squad run. Used when an external flow task is held for approval (durable spec 5.4).
	SkipAutoEnqueue bool
}
