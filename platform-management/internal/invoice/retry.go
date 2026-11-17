package invoice

import (
	"errors"
	"time"
)

const (
	backoffBase = time.Minute
	backoffCap  = 6 * time.Hour
)

func Backoff(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	// Cap the exponent before shifting so large attempt counts cannot overflow.
	if attempts > 10 {
		return backoffCap
	}
	d := backoffBase << (attempts - 1)
	if d > backoffCap {
		return backoffCap
	}
	return d
}

type Decision struct {
	State State
	Delay time.Duration
}

func Decide(err error, attempts, maxAttempts int) Decision {
	var pe *ProviderError
	if errors.As(err, &pe) {
		switch pe.Kind {
		case KindMissingFields:
			return Decision{State: StateNeedsInfo}
		case KindPermanent:
			return Decision{State: StateFailed}
		}
	}
	if attempts >= maxAttempts {
		return Decision{State: StateFailed}
	}
	return Decision{State: StatePending, Delay: Backoff(attempts)}
}
