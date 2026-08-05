package application

import "testing"

func TestAppTemplatesExposeFiveFixedCreationModes(t *testing.T) {
	templates := ListAppTemplates()
	if len(templates) != 5 {
		t.Fatalf("template count = %d, want 5", len(templates))
	}
	dynamic, ok := FindAppTemplate(string(AppTemplateDynamicComposition))
	if !ok || dynamic.PromptStrategy != PromptStrategyBoundExact {
		t.Fatalf("dynamic template = %#v", dynamic)
	}
	for _, capability := range []AppCapability{AppCapabilityChat, AppCapabilityImageGeneration, AppCapabilityImageEdit} {
		if !dynamic.AllowsCapability(capability) {
			t.Fatalf("dynamic template should allow %s", capability)
		}
	}
}
