package main

import "testing"

func TestAIModulesLifecycleIsSafeForEmptyBundle(t *testing.T) {
	modules := &aiModules{}
	modules.Start(nil)
	modules.Start(nil)
	modules.Stop(nil)
	modules.Stop(nil)
}
