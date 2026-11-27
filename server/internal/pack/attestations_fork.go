package pack

import (
	"fmt"

	"github.com/multica-ai/multica/server/internal/hub"
)

// readPackAttestations collects attestations/*.json carried by a Hub agent template bundle
// (durable spec 13.3, F12). Malformed JSON refuses the bundle with ErrInvalid.
func readPackAttestations(b *hub.Bundle, p *Pack) error {
	atts, err := hub.ExtractAttestations(b)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	p.Attestations = atts
	return nil
}
