package postgres

import (
	"xiaodou/dai/internal/ai/adapters/bridgefmt"
	corebridge "xiaodou/dai/internal/ai/core/bridge"
	"xiaodou/dai/internal/ai/core/catalog"
	"xiaodou/dai/internal/ai/core/surface"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/runtimecompat"
)

var defaultBridgeSupport corebridge.SupportMatrix = bridgefmt.NewRuntime()

func normalizeBridgeSupport(support corebridge.SupportMatrix) corebridge.SupportMatrix {
	if support != nil {
		return support
	}
	return defaultBridgeSupport
}

func chooseProviderProtocolWithSupport(
	support corebridge.SupportMatrix,
	capType domain.CapabilityType,
	clientProtocol domain.UpstreamProtocol,
	supported []domain.UpstreamProtocol,
	allowConversion, isStream bool,
) (provider domain.UpstreamProtocol, bucket int, ok bool) {
	// 零转换 passthrough 恒为最优（桶 0）：候选直接支持 client 协议本身。
	for _, p := range supported {
		if p == clientProtocol {
			return p, 0, true
		}
	}

	// 需转换：仅在分组开关开启时才考虑；可转换性和偏好完全由真实 bridge runtime 判定。
	if !allowConversion {
		return "", 0, false
	}

	capability := runtimecompat.CapabilityToCore(capType)
	clientSurface, err := runtimecompat.ProtocolToSurfaceForCapability(clientProtocol, capability)
	if err != nil {
		return "", 0, false
	}
	support = normalizeBridgeSupport(support)

	const unset = 1 << 30
	bestBucket, bestPrio := unset, unset
	for _, p := range supported {
		providerSurface, err := runtimecompat.ProtocolToSurfaceForCapability(p, capability)
		if err != nil {
			continue
		}
		b, prio, prefOK := support.PreferenceForCapability(capability, clientSurface, providerSurface, isStream)
		if !prefOK {
			continue
		}
		if b < bestBucket || (b == bestBucket && prio < bestPrio) {
			bestBucket, bestPrio, provider = b, prio, p
		}
	}
	if provider == "" {
		return "", 0, false
	}
	return provider, bestBucket, true
}

func bridgeSurfaceSupportedForCapability(
	support corebridge.SupportMatrix,
	clientSurface, providerSurface surface.ID,
	capability catalog.Capability,
	stream bool,
) bool {
	support = normalizeBridgeSupport(support)
	if !support.NeedsBridge(clientSurface, providerSurface) {
		return true
	}
	_, _, ok := support.PreferenceForCapability(capability, clientSurface, providerSurface, stream)
	return ok
}
