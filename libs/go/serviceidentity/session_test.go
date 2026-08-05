package serviceidentity

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestManagerRegistersWithoutSecretAndTracksReadiness(t *testing.T) {
	var calls atomic.Int32
	manager, err := NewManager(SessionConfig{ServiceBaseURL: "http://urm-service.test", ServiceID: "ai-service", InstanceID: "instance-1"})
	if err != nil {
		t.Fatal(err)
	}
	manager.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		if req.URL.Path != "/service/v1/sessions" {
			t.Fatalf("path = %s", req.URL.Path)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewBufferString(`{"access_token":"token","token_type":"Bearer","expires_in":300,"source_cidr":"10.0.0.7/32","service_id":"ai-service","instance_id":"instance-1"}`))}, nil
	})
	token, err := manager.Token(context.Background())
	if err != nil || token != "token" {
		t.Fatalf("token=%q err=%v", token, err)
	}
	if ready, err := manager.Ready(); !ready || err != nil {
		t.Fatalf("ready=%v err=%v", ready, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls=%d", calls.Load())
	}
}

func TestManagerRenewFailureDegradesReadinessButKeepsLiveToken(t *testing.T) {
	manager, _ := NewManager(SessionConfig{ServiceBaseURL: "http://urm-service.test", ServiceID: "proxy-service"})
	manager.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("URM unavailable") })
	manager.mu.Lock()
	manager.token = "still-valid"
	manager.expiresAt = time.Now().Add(time.Minute)
	manager.ready = true
	manager.mu.Unlock()
	manager.renew(context.Background())
	if ready, err := manager.Ready(); ready || err == nil {
		t.Fatalf("ready=%v err=%v", ready, err)
	}
	if token, err := manager.Token(context.Background()); token != "still-valid" || err != nil {
		t.Fatalf("token=%q err=%v", token, err)
	}
}

func TestManagerUsesStableInstanceIdentity(t *testing.T) {
	explicit, err := NewManager(SessionConfig{ServiceBaseURL: "http://urm-service.test", ServiceID: "ai-service", InstanceID: " instance-1 "})
	if err != nil {
		t.Fatal(err)
	}
	if got := explicit.InstanceID(); got != "instance-1" {
		t.Fatalf("explicit instance ID = %q", got)
	}

	first, err := NewManager(SessionConfig{ServiceBaseURL: "http://urm-service.test", ServiceID: "ai-service"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewManager(SessionConfig{ServiceBaseURL: "http://urm-service.test", ServiceID: "ai-service"})
	if err != nil {
		t.Fatal(err)
	}
	if first.InstanceID() == "" || first.InstanceID() != second.InstanceID() {
		t.Fatalf("default instance IDs are not stable: %q, %q", first.InstanceID(), second.InstanceID())
	}
}

func TestInstanceIDFromHostnameIsBoundedAndDeterministic(t *testing.T) {
	hostname := strings.Repeat("a", maxInstanceIDLength+1)
	first := instanceIDFromHostname(hostname)
	second := instanceIDFromHostname(hostname)
	if first != second || !strings.HasPrefix(first, "host-") || len(first) > maxInstanceIDLength {
		t.Fatalf("hashed hostname instance ID = %q, second = %q", first, second)
	}

	_, err := NewManager(SessionConfig{
		ServiceBaseURL: "http://urm-service.test",
		ServiceID:      "ai-service",
		InstanceID:     strings.Repeat("b", maxInstanceIDLength+1),
	})
	if err == nil {
		t.Fatal("expected an overlong explicit instance ID to fail")
	}
}
