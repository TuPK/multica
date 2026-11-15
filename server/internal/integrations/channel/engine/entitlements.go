package engine

import (
	"sync/atomic"

	"github.com/multica-ai/multica/server/internal/entitlement"
)

// The media bind reads the workspace's storage quota from the entitlement
// provider. The chat sessions are built in five integrations, so the provider is
// set once at startup instead of threaded through each constructor. Unset, or
// set to nil, every gate is off.
type providerBox struct{ provider entitlement.Provider }

var entitlements atomic.Pointer[providerBox]

// SetEntitlements sets the provider the media bind reads its storage quota from.
func SetEntitlements(provider entitlement.Provider) {
	entitlements.Store(&providerBox{provider: provider})
}

func currentEntitlements() entitlement.Provider {
	if box := entitlements.Load(); box != nil {
		return box.provider
	}
	return nil
}
