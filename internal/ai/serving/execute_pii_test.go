package serving

import (
	"bytes"
	"context"
	"testing"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/privacy"
)

type configuredPIIGate struct {
	active    bool
	protector *privacy.Protector
}

func (g configuredPIIGate) IsActive(context.Context, string) (bool, error) {
	return false, nil
}

func (g configuredPIIGate) PIIProtection(context.Context) (bool, *privacy.Protector, error) {
	return g.active, g.protector, nil
}

func TestBuildUpstreamBodyUsesConfiguredPIIProtection(t *testing.T) {
	protector, err := privacy.NewProtectorWithConfig(privacy.Config{
		PlaceholderPrefix: "SECURE",
		Rules: []privacy.RuleConfig{
			{ID: "employee", Name: "员工编号", Pattern: `EMP-[0-9]{4}`, Enabled: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := &Request{
		Envelope:       &RequestEnvelope{ClientBody: []byte(`{"model":"public-model","messages":[{"role":"user","content":"employee EMP-1234"}]}`)},
		ClientProtocol: domain.ProtocolOpenAIChat,
		Candidate:      &domain.RouteCandidate{Protocol: domain.ProtocolOpenAIChat},
	}
	step := &ExecuteStep{
		Bridge:     testProtocolBridge{},
		ModuleGate: configuredPIIGate{active: true, protector: protector},
		Privacy:    privacy.NewProtector(),
	}

	prepared, err := step.buildUpstreamBody(req)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(prepared.Body, []byte("EMP-1234")) {
		t.Fatalf("configured sensitive value reached upstream: %s", prepared.Body)
	}
	if !bytes.Contains(prepared.Body, []byte("__SECURE_PII_EMPLOYEE_1__")) {
		t.Fatalf("configured placeholder missing from upstream body: %s", prepared.Body)
	}
}
