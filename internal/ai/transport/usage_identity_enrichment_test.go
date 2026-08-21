package transport

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestIdentityEnrichmentFailuresAreObservedAndFailOpen(t *testing.T) {
	observer := &identityEnrichmentFailureObserverStub{events: make(map[string]string)}
	included := buildIdentityIncluded(t.Context(), failingIdentityProvider{}, observer, []string{"user-1"}, []string{"tenant-1"})
	if len(included.Users) != 0 || len(included.Tenants) != 0 {
		t.Fatalf("included = %#v, want empty fail-open result", included)
	}
	observer.mu.Lock()
	defer observer.mu.Unlock()
	if observer.events["users"] != "users unavailable" || observer.events["tenants"] != "tenants unavailable" {
		t.Fatalf("events = %#v", observer.events)
	}
}

type failingIdentityProvider struct{}

func (failingIdentityProvider) BatchGetUsers(context.Context, []string) (map[string]*IdentityUser, error) {
	return nil, errors.New("users unavailable")
}

func (failingIdentityProvider) BatchGetTenants(context.Context, []string) (map[string]*IdentityTenant, error) {
	return nil, errors.New("tenants unavailable")
}

type identityEnrichmentFailureObserverStub struct {
	mu     sync.Mutex
	events map[string]string
}

func (s *identityEnrichmentFailureObserverStub) ObserveFailure(kind string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events[kind] = err.Error()
}
