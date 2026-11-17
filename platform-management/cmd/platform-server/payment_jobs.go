package main

import (
	"context"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/invoice"
	"github.com/multica-ai/multica/platform-management/internal/jobs"
)

const (
	paymentNoticeRedriveJob   = "payment_notice_redrive"
	paymentNoticeRedriveEvery = 5 * time.Minute
)

// registerPaymentJobs adds the payment-notice redrive and the two e-invoice
// jobs (spec 7.2, 7.3 rule 3) and returns the first registration error.
func registerPaymentJobs(runner *jobs.Runner, reconciler *billing.Reconciler, einvoices *invoice.Service) error {
	if err := runner.Register(paymentNoticeRedriveJob, paymentNoticeRedriveEvery, func(ctx context.Context) error {
		_, err := reconciler.Redrive(ctx, billing.RedriveAge, billing.RedriveLimit)
		return err
	}); err != nil {
		return err
	}
	if err := runner.RegisterEinvoiceEnqueue(func(ctx context.Context) error {
		_, err := einvoices.Enqueue(ctx)
		return err
	}); err != nil {
		return err
	}
	return runner.RegisterEinvoiceIssue(func(ctx context.Context) error {
		_, err := einvoices.Issue(ctx)
		return err
	})
}
