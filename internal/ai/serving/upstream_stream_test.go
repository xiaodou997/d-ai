package serving

import (
	"testing"

	"xiaodou/dai/internal/ai/domain"
)

func TestRequestUpstreamStreamDecoupling(t *testing.T) {
	img := func(mode string) *domain.RouteCandidate {
		return &domain.RouteCandidate{ImageStreamMode: mode}
	}
	cases := []struct {
		name       string
		capability domain.CapabilityType
		clientWant bool
		candidate  *domain.RouteCandidate
		want       bool
	}{
		// 图像：binding force_sync/force_stream 无视客户端意愿。
		{"image force_sync overrides client stream", domain.CapabilityImage, true, img(domain.ImageStreamModeForceSync), false},
		{"image force_stream overrides client sync", domain.CapabilityImage, false, img(domain.ImageStreamModeForceStream), true},
		{"image auto follows client stream", domain.CapabilityImage, true, img(domain.ImageStreamModeAuto), true},
		{"image auto follows client sync", domain.CapabilityImage, false, img(domain.ImageStreamModeAuto), false},
		// 非图像：始终跟随客户端。
		{"chat follows client stream", domain.CapabilityChat, true, img(domain.ImageStreamModeForceSync), true},
		{"chat follows client sync", domain.CapabilityChat, false, img(domain.ImageStreamModeForceStream), false},
		// 图像但无候选：回退客户端。
		{"image no candidate follows client", domain.CapabilityImage, true, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := &Request{CapabilityType: tc.capability, IsStream: tc.clientWant, Candidate: tc.candidate}
			if got := req.UpstreamStream(); got != tc.want {
				t.Fatalf("UpstreamStream() = %v, want %v", got, tc.want)
			}
		})
	}
}
