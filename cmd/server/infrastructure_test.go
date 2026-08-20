package main

import (
	"context"
	"testing"
)

func TestOpenInfrastructureRejectsNilConfiguration(t *testing.T) {
	if _, err := openInfrastructure(context.Background(), nil, nil); err == nil {
		t.Fatal("openInfrastructure(nil) returned nil error")
	}
}
