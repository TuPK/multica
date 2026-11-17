package paymentnotice

import "context"

type Store interface {
	// Insert stores a notice in state received. When the provider event is
	// already stored it returns the existing row and inserted=false.
	Insert(ctx context.Context, n NewNotice) (notice Notice, inserted bool, err error)
	Get(ctx context.Context, id string) (Notice, error)
	// Transition moves a notice from Change.From to Change.To.
	Transition(ctx context.Context, c Change) error
	// ListNeedsReview returns the review queue, oldest first.
	ListNeedsReview(ctx context.Context, limit int) ([]Notice, error)
}
