package bridge

import (
	"xiaodou/dai/internal/ai/core/catalog"
	"xiaodou/dai/internal/ai/core/surface"
)

// SupportMatrix exposes routing-time bridge support and preference checks
// backed by the concrete runtime bridge implementation.
type SupportMatrix interface {
	NeedsBridge(clientSurface, providerSurface surface.ID) bool
	PreferenceForCapability(capability catalog.Capability, clientSurface, providerSurface surface.ID, stream bool) (bucket, priority int, ok bool)
}
