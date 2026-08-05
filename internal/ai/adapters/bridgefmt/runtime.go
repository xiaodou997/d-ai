package bridgefmt

import (
	"fmt"

	corebridge "xiaodou/dai/internal/ai/core/bridge"
	"xiaodou/dai/internal/ai/core/catalog"
	"xiaodou/dai/internal/ai/core/surface"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/formats"
)

// Runtime adapts the legacy formats registry to the core bridge runtime port.
type requestBridgeFunc func(corebridge.RequestEnvelope, []byte) ([]byte, error)
type responseBridgeFunc func(corebridge.ResponseEnvelope, []byte) ([]byte, error)
type streamProviderFactory func(corebridge.ResponseEnvelope, string) (corebridge.StreamProvider, error)
type streamEmitterFactory func(corebridge.ResponseEnvelope) (corebridge.StreamEmitter, error)

type Runtime struct {
	registry         *corebridge.Registry
	requestHandlers  map[corebridge.Definition]requestBridgeFunc
	responseHandlers map[corebridge.Definition]responseBridgeFunc
	streamProviders  map[corebridge.Definition]streamProviderFactory
	streamEmitters   map[corebridge.Definition]streamEmitterFactory
}

var _ corebridge.SupportMatrix = (*Runtime)(nil)

func NewRuntime() *Runtime {
	r := &Runtime{
		registry:         corebridge.NewRegistry(),
		requestHandlers:  map[corebridge.Definition]requestBridgeFunc{},
		responseHandlers: map[corebridge.Definition]responseBridgeFunc{},
		streamProviders:  map[corebridge.Definition]streamProviderFactory{},
		streamEmitters:   map[corebridge.Definition]streamEmitterFactory{},
	}
	r.registerTextBridges()
	r.registerEmbeddingBridges()
	r.registerImageBridges()
	return r
}

func (r *Runtime) ConvertRequest(env corebridge.RequestEnvelope, body []byte) ([]byte, error) {
	if handler, ok := r.lookupRequestHandler(env.Kind, env.Source, env.Target); ok {
		return handler(env, body)
	}
	return nil, fmt.Errorf("bridgefmt: unsupported bridge request %q -> %q (kind %q)", env.Source, env.Target, env.Kind)
}

func (r *Runtime) ConvertResponse(env corebridge.ResponseEnvelope, body []byte) ([]byte, error) {
	if handler, ok := r.lookupResponseHandler(env.Kind, env.Source, env.Target); ok {
		return handler(env, body)
	}
	return nil, fmt.Errorf("bridgefmt: unsupported bridge response %q -> %q (kind %q)", env.Source, env.Target, env.Kind)
}

func (r *Runtime) NeedsBridge(clientSurface, providerSurface surface.ID) bool {
	return clientSurface != providerSurface
}

func (r *Runtime) PreferenceForCapability(
	capability catalog.Capability,
	clientSurface, providerSurface surface.ID,
	stream bool,
) (bucket, priority int, ok bool) {
	if !r.NeedsBridge(clientSurface, providerSurface) {
		return 0, 0, true
	}
	kind := bridgeKindForCapability(capability)
	if _, ok := r.lookupRequestHandler(kind, clientSurface, providerSurface); !ok {
		return 0, 0, false
	}
	switch kind {
	case corebridge.IRKindChat:
		if stream {
			if _, ok := r.lookupStreamProviderFactory(kind, providerSurface, clientSurface); !ok {
				return 0, 0, false
			}
			if _, ok := r.lookupStreamEmitterFactory(kind, providerSurface, clientSurface); !ok {
				return 0, 0, false
			}
		} else {
			if _, ok := r.lookupResponseHandler(kind, providerSurface, clientSurface); !ok {
				return 0, 0, false
			}
		}
		return textSurfacePreference(clientSurface, providerSurface)
	case corebridge.IRKindImage:
		if _, ok := r.lookupResponseHandler(kind, providerSurface, clientSurface); !ok {
			return 0, 0, false
		}
		return pairPreference(clientSurface, providerSurface, []surface.ID{
			surface.GeminiImages,
			surface.OpenAIImages,
		})
	case corebridge.IRKindEmbedding:
		if stream {
			return 0, 0, false
		}
		if _, ok := r.lookupResponseHandler(kind, providerSurface, clientSurface); !ok {
			return 0, 0, false
		}
		return pairPreference(clientSurface, providerSurface, []surface.ID{
			surface.GeminiEmbeddings,
			surface.OpenAIEmbeddings,
		})
	default:
		return 0, 0, false
	}
}

func bridgeKindForCapability(capability catalog.Capability) corebridge.IRKind {
	switch capability {
	case catalog.CapabilityImageGeneration, catalog.CapabilityImageEdit:
		return corebridge.IRKindImage
	case catalog.CapabilityEmbedding:
		return corebridge.IRKindEmbedding
	default:
		return corebridge.IRKindChat
	}
}

func isImageCapability(capability catalog.Capability) bool {
	return bridgeKindForCapability(capability) == corebridge.IRKindImage
}

func (r *Runtime) registerTextBridges() {
	textSurfaces := []surface.ID{
		surface.OpenAIChat,
		surface.OpenAIResponses,
		surface.AnthropicMessages,
		surface.GeminiText,
	}
	for _, source := range textSurfaces {
		for _, target := range textSurfaces {
			if source == target {
				continue
			}
			def := corebridge.Definition{Kind: corebridge.IRKindChat, Source: source, Target: target}
			request := func(env corebridge.RequestEnvelope, body []byte) ([]byte, error) {
				src, err := formatIDForSurface(env.Source)
				if err != nil {
					return nil, err
				}
				dst, err := formatIDForSurface(env.Target)
				if err != nil {
					return nil, err
				}
				return formats.ConvertRequest(src, dst, body, env.TargetModel, env.Stream)
			}
			response := func(env corebridge.ResponseEnvelope, body []byte) ([]byte, error) {
				src, err := formatIDForSurface(env.Source)
				if err != nil {
					return nil, err
				}
				dst, err := formatIDForSurface(env.Target)
				if err != nil {
					return nil, err
				}
				return formats.ConvertResponse(src, dst, body)
			}
			streamProvider := func(env corebridge.ResponseEnvelope, fallbackModel string) (corebridge.StreamProvider, error) {
				src, err := formatIDForSurface(env.Source)
				if err != nil {
					return nil, err
				}
				provider, err := formats.NewStreamProvider(src, fallbackModel)
				if err != nil {
					return nil, err
				}
				return &streamProviderAdapter{inner: provider}, nil
			}
			streamEmitter := func(env corebridge.ResponseEnvelope) (corebridge.StreamEmitter, error) {
				dst, err := formatIDForSurface(env.Target)
				if err != nil {
					return nil, err
				}
				emitter, err := formats.NewStreamEmitter(dst)
				if err != nil {
					return nil, err
				}
				return &streamEmitterAdapter{inner: emitter}, nil
			}
			r.registerBridge(def, request, response, streamProvider, streamEmitter)
		}
	}
}

func (r *Runtime) registerBridge(
	def corebridge.Definition,
	request requestBridgeFunc,
	response responseBridgeFunc,
	streamProvider streamProviderFactory,
	streamEmitter streamEmitterFactory,
) {
	if r == nil {
		return
	}
	if !r.registry.Supports(def.Kind, def.Source, def.Target) {
		r.registry.Register(def)
	}
	if request != nil {
		r.requestHandlers[def] = request
	}
	if response != nil {
		r.responseHandlers[def] = response
	}
	if streamProvider != nil {
		r.streamProviders[def] = streamProvider
	}
	if streamEmitter != nil {
		r.streamEmitters[def] = streamEmitter
	}
}

func (r *Runtime) lookupRequestHandler(kind corebridge.IRKind, source, target surface.ID) (requestBridgeFunc, bool) {
	if r == nil || r.registry == nil {
		return nil, false
	}
	def := corebridge.Definition{Kind: kind, Source: source, Target: target}
	if !r.registry.Supports(def.Kind, def.Source, def.Target) {
		return nil, false
	}
	handler, ok := r.requestHandlers[def]
	return handler, ok
}

func (r *Runtime) lookupResponseHandler(kind corebridge.IRKind, source, target surface.ID) (responseBridgeFunc, bool) {
	if r == nil || r.registry == nil {
		return nil, false
	}
	def := corebridge.Definition{Kind: kind, Source: source, Target: target}
	if !r.registry.Supports(def.Kind, def.Source, def.Target) {
		return nil, false
	}
	handler, ok := r.responseHandlers[def]
	return handler, ok
}

func (r *Runtime) NewStreamProvider(env corebridge.ResponseEnvelope, fallbackModel string) (corebridge.StreamProvider, error) {
	if factory, ok := r.lookupStreamProviderFactory(env.Kind, env.Source, env.Target); ok {
		return factory(env, fallbackModel)
	}
	return nil, fmt.Errorf("bridgefmt: unsupported stream provider %q -> %q (kind %q)", env.Source, env.Target, env.Kind)
}

func (r *Runtime) NewStreamEmitter(env corebridge.ResponseEnvelope) (corebridge.StreamEmitter, error) {
	if factory, ok := r.lookupStreamEmitterFactory(env.Kind, env.Source, env.Target); ok {
		return factory(env)
	}
	return nil, fmt.Errorf("bridgefmt: unsupported stream emitter %q -> %q (kind %q)", env.Source, env.Target, env.Kind)
}

func (r *Runtime) lookupStreamProviderFactory(kind corebridge.IRKind, source, target surface.ID) (streamProviderFactory, bool) {
	if r == nil || r.registry == nil {
		return nil, false
	}
	def := corebridge.Definition{Kind: kind, Source: source, Target: target}
	if !r.registry.Supports(def.Kind, def.Source, def.Target) {
		return nil, false
	}
	factory, ok := r.streamProviders[def]
	return factory, ok
}

func (r *Runtime) lookupStreamEmitterFactory(kind corebridge.IRKind, source, target surface.ID) (streamEmitterFactory, bool) {
	if r == nil || r.registry == nil {
		return nil, false
	}
	def := corebridge.Definition{Kind: kind, Source: source, Target: target}
	if !r.registry.Supports(def.Kind, def.Source, def.Target) {
		return nil, false
	}
	factory, ok := r.streamEmitters[def]
	return factory, ok
}

func textSurfacePreference(client, provider surface.ID) (bucket, priority int, ok bool) {
	clientFmt, err := formatIDForSurface(client)
	if err != nil {
		return 0, 0, false
	}
	providerFmt, err := formatIDForSurface(provider)
	if err != nil {
		return 0, 0, false
	}
	return formats.CandidatePreference(clientFmt, providerFmt)
}

func pairPreference(client, provider surface.ID, ordered []surface.ID) (bucket, priority int, ok bool) {
	if client == provider {
		return 0, 0, true
	}
	priority = surfacePriority(ordered, provider)
	if priority < 0 {
		return 0, 0, false
	}
	return 1, priority, true
}

func surfacePriority(ordered []surface.ID, target surface.ID) int {
	for idx, id := range ordered {
		if id == target {
			return idx
		}
	}
	return -1
}

func formatIDForSurface(id surface.ID) (formats.FormatID, error) {
	switch id {
	case surface.OpenAIChat:
		return formats.FormatOpenAIChat, nil
	case surface.OpenAIResponses:
		return formats.FormatOpenAIResponses, nil
	case surface.AnthropicMessages:
		return formats.FormatClaudeMessages, nil
	case surface.GeminiText:
		return formats.FormatGeminiGenerate, nil
	default:
		return "", fmt.Errorf("bridgefmt: unsupported surface %q", id)
	}
}

func (r *Runtime) RewritePassthroughBody(body []byte, targetModel, contentType string, fixedProviderType domain.FixedProviderType) ([]byte, error) {
	_ = fixedProviderType
	return formats.RewriteModel(body, targetModel, contentType)
}

func (r *Runtime) ExtractSyncUsageForProtocol(protocol domain.UpstreamProtocol, body []byte) domain.TokenUsage {
	return formats.ExtractSyncUsage(body, protocol)
}

func (r *Runtime) ExtractStreamUsageForProtocol(protocol domain.UpstreamProtocol, prev domain.TokenUsage, data []byte, eventType string) (domain.TokenUsage, bool) {
	return formats.ExtractStreamUsage(prev, data, eventType, protocol)
}

func (r *Runtime) BuildStreamErrorFrame(protocol domain.UpstreamProtocol, code, msg string) []byte {
	return formats.StreamErrorFrame(protocol, code, msg)
}

type streamProviderAdapter struct {
	inner formats.StreamProvider
}

func (a *streamProviderAdapter) PushLine(line []byte) ([]corebridge.StreamFrame, error) {
	frames, err := a.inner.PushLine(line)
	if err != nil {
		return nil, err
	}
	return mapStreamFrames(frames), nil
}

func (a *streamProviderAdapter) Finish() ([]corebridge.StreamFrame, error) {
	frames, err := a.inner.Finish()
	if err != nil {
		return nil, err
	}
	return mapStreamFrames(frames), nil
}

type streamEmitterAdapter struct {
	inner formats.StreamEmitter
}

func (a *streamEmitterAdapter) Emit(frame corebridge.StreamFrame) ([]byte, error) {
	return a.inner.Emit(mapStreamFrame(frame))
}

func (a *streamEmitterAdapter) Finish() ([]byte, error) {
	return a.inner.Finish()
}

func mapStreamFrames(in []formats.StreamFrame) []corebridge.StreamFrame {
	out := make([]corebridge.StreamFrame, 0, len(in))
	for _, frame := range in {
		out = append(out, mapStreamFrameOut(frame))
	}
	return out
}

func mapStreamFrameOut(in formats.StreamFrame) corebridge.StreamFrame {
	return corebridge.StreamFrame{
		ID:           in.ID,
		Model:        in.Model,
		Event:        corebridge.StreamEvent(in.Event),
		Text:         in.Text,
		ToolIndex:    in.ToolIndex,
		CallID:       in.CallID,
		Name:         in.Name,
		Arguments:    in.Arguments,
		ToolUseID:    in.ToolUseID,
		Content:      in.Content,
		FinishReason: in.FinishReason,
		HasFinish:    in.HasFinish,
		Usage:        mapUsageOut(in.Usage),
		Unknown:      append([]byte(nil), in.Unknown...),
	}
}

func mapStreamFrame(in corebridge.StreamFrame) formats.StreamFrame {
	return formats.StreamFrame{
		ID:           in.ID,
		Model:        in.Model,
		Event:        formats.StreamEvent(in.Event),
		Text:         in.Text,
		ToolIndex:    in.ToolIndex,
		CallID:       in.CallID,
		Name:         in.Name,
		Arguments:    in.Arguments,
		ToolUseID:    in.ToolUseID,
		Content:      in.Content,
		FinishReason: in.FinishReason,
		HasFinish:    in.HasFinish,
		Usage:        mapUsageIn(in.Usage),
		Unknown:      append([]byte(nil), in.Unknown...),
	}
}

func mapUsageOut(in *formats.Usage) *corebridge.Usage {
	if in == nil {
		return nil
	}
	return &corebridge.Usage{
		InputTokens:                 in.InputTokens,
		OutputTokens:                in.OutputTokens,
		TotalTokens:                 in.TotalTokens,
		CacheReadTokens:             in.CacheReadTokens,
		CacheWriteTokens:            in.CacheWriteTokens,
		CacheCreationEphemeral5mTok: in.CacheCreationEphemeral5mTok,
		CacheCreationEphemeral1hTok: in.CacheCreationEphemeral1hTok,
		ReasoningTokens:             in.ReasoningTokens,
	}
}

func mapUsageIn(in *corebridge.Usage) *formats.Usage {
	if in == nil {
		return nil
	}
	return &formats.Usage{
		InputTokens:                 in.InputTokens,
		OutputTokens:                in.OutputTokens,
		TotalTokens:                 in.TotalTokens,
		CacheReadTokens:             in.CacheReadTokens,
		CacheWriteTokens:            in.CacheWriteTokens,
		CacheCreationEphemeral5mTok: in.CacheCreationEphemeral5mTok,
		CacheCreationEphemeral1hTok: in.CacheCreationEphemeral1hTok,
		ReasoningTokens:             in.ReasoningTokens,
	}
}
