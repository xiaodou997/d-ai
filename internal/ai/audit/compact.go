package audit

import "xiaodou/dai/internal/ai/domain"

type CompactionMetadata struct {
	Truncated     bool     `json:"truncated"`
	OriginalBytes int      `json:"original_bytes"`
	OmittedFields []string `json:"omitted_fields,omitempty"`
}

// CompactTextPayload is shared by both enqueue paths. Preserve diagnostic
// fields while dropping oversized text bodies; never cut JSON mid-value.
func CompactTextPayload(p *Payload) {
	if p == nil || p.Compaction.Truncated {
		return
	}
	protocol := domain.UpstreamProtocol(p.ClientProtocol)
	if protocol == domain.ProtocolOpenAIImages || p.CapabilityType == string(domain.CapabilityImage) || p.CapabilityType == string(domain.CapabilityVideo) || p.CapabilityType == string(domain.CapabilityAudioTTS) || p.CapabilityType == string(domain.CapabilityAudioSTT) {
		return
	}
	sz := payloadSize(p)
	if sz <= workerMaxPayload {
		return
	}
	p.Compaction = CompactionMetadata{Truncated: true, OriginalBytes: sz}
	for _, field := range []struct {
		name  string
		clear func()
		size  int
	}{
		{"request_messages", func() { p.RequestMessages = nil }, len(p.RequestMessages)},
		{"request_params", func() { p.RequestParams = nil }, len(p.RequestParams)},
		{"response_message", func() { p.ResponseMessage = nil }, len(p.ResponseMessage)},
		{"media_refs", func() { p.MediaRefs = nil }, len(p.MediaRefs)},
		{"request_headers", func() { p.RequestHeaders = nil }, len(p.RequestHeaders)},
		{"response_headers", func() { p.ResponseHeaders = nil }, len(p.ResponseHeaders)},
	} {
		if field.size > 0 {
			field.clear()
			p.Compaction.OmittedFields = append(p.Compaction.OmittedFields, field.name)
		}
	}
}
