package jobs

import (
	"context"
	"time"
)

// AccountRequestFinalizeJob is the name of step 3 of an account request (spec
// 7b.5, rule 7): the platform writes the billing profile and the subscription
// of each request that Delegora has provisioned, then sends the welcome.
const AccountRequestFinalizeJob = "account_request_finalize"

// AccountRequestFinalizeEvery is the job's interval. A failed request is tried
// again at the next run, so retries are at least this far apart.
const AccountRequestFinalizeEvery = time.Minute

// RegisterAccountRequestFinalize adds the finalize job, which runs fn.
func (r *Runner) RegisterAccountRequestFinalize(fn func(context.Context) error) error {
	return r.Register(AccountRequestFinalizeJob, AccountRequestFinalizeEvery, fn)
}
