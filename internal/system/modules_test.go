package system

import "testing"

func TestDefinitionsExposeDedicatedConfigurationRoutes(t *testing.T) {
	routes := make(map[string]string)
	for _, definition := range Definitions() {
		routes[definition.Name] = definition.AdminRoute
	}

	if got := routes[ModuleProxyEgress]; got != "/admin/proxy-nodes" {
		t.Fatalf("proxy egress admin route = %q", got)
	}
	if got := routes[ModulePII]; got != "/admin/system-modules/pii-protection" {
		t.Fatalf("pii protection admin route = %q", got)
	}
}
