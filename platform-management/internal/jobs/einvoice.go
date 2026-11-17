package jobs

import (
	"context"
	"time"
)

// EinvoiceEnqueueJob creates an issuance record for each paid order that has
// none (spec 7.3, rule 3).
const EinvoiceEnqueueJob = "einvoice_enqueue"

// EinvoiceIssueJob sends the due records to the e-invoice provider.
const EinvoiceIssueJob = "einvoice_issue"

// EinvoiceEvery is the interval of both jobs. Retry delays come from the
// record's next attempt time, so a run only picks up what is due.
const EinvoiceEvery = time.Minute

// RegisterEinvoiceEnqueue adds the enqueue job, which runs fn.
func (r *Runner) RegisterEinvoiceEnqueue(fn func(context.Context) error) error {
	return r.Register(EinvoiceEnqueueJob, EinvoiceEvery, fn)
}

// RegisterEinvoiceIssue adds the issue job, which runs fn.
func (r *Runner) RegisterEinvoiceIssue(fn func(context.Context) error) error {
	return r.Register(EinvoiceIssueJob, EinvoiceEvery, fn)
}
