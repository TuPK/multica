package service

import (
	"context"

	"github.com/multica-ai/multica/server/internal/attribution"
)

// RecordHumanDelegation is a deliberate no-op. Upstream calls it to feed the
// self-host telemetry ledger, which this fork removed (docs/fork-license-telemetry.md).
// It stays so a new upstream call site compiles unchanged and reports nothing.
func (s *TaskService) RecordHumanDelegation(_ context.Context, _ attribution.Result) {}
