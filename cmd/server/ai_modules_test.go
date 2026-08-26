package main

import (
	"context"
	"testing"
)

func TestAIModulesLifecycleIsSafeForEmptyBundle(t *testing.T) {
	modules := &aiModules{}
	modules.Start(context.Background())
	modules.Start(context.Background())
	modules.Stop(context.Background())
	modules.Stop(context.Background())
}
