package issueworkflow

// DocTaskFork holds fork-specific task fields.
type DocTaskFork struct {
	// Effects marks work whose result lands outside the platform. "external" holds the
	// task until a person approves it (durable spec 5.4). Empty is ordinary work.
	Effects string `json:"effects,omitempty"`
}
