package main

import "testing"

func TestConfigureSecretKeyringRequiresConfig(t *testing.T) {
	if err := configureSecretKeyring(nil); err == nil {
		t.Fatal("expected nil configuration to be rejected")
	}
}

func TestPlatformModulesLifecycleIsSafeForEmptyBundle(t *testing.T) {
	modules := &platformModules{}
	modules.Start()
	modules.Start()
	modules.Stop()
	modules.Stop()
}
