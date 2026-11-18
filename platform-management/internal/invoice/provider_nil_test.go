package invoice

import (
	"context"
	"errors"
	"testing"
)

// A service without a provider must answer ErrProviderNotConfigured before it
// touches the store: the store is nil here, so any store call would panic.
func TestNilProviderRetryReturnsSentinelBeforeStore(t *testing.T) {
	s := NewService(nil, nil, Config{})
	ctx := context.Background()
	if _, err := s.Retry(ctx, "order", 1, "actor"); !errors.Is(err, ErrProviderNotConfigured) {
		t.Errorf("retry error = %v, want ErrProviderNotConfigured", err)
	}
}
