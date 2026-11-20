// Package legalhold is the server's view of a legal hold (spec R6 section 3c.7,
// D104). The platform places and releases a hold; Multica refuses every deletion
// it covers. A held object keeps its data, and ordinary operation is unaffected.
package legalhold

import "errors"

// Code is the error code of a deletion that a hold blocks: 409 for the owner's
// request, and error_code of a failed staff request.
const Code = "legal_hold"

// ErrHeld matches every refusal a legal hold causes.
var ErrHeld = errors.New(Code)

// Blocked is the refusal of a deletion that a live hold covers. HoldID is kept
// for the audit row only; the API answers with Code and nothing else.
type Blocked struct{ HoldID string }

func (b *Blocked) Error() string { return Code + ": hold " + b.HoldID }

// Is makes errors.Is(err, ErrHeld) hold for every Blocked error.
func (b *Blocked) Is(target error) bool { return target == ErrHeld }
