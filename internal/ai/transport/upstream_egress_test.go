package transport

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync/atomic"
	"testing"
)

func newPrivateNetworkTestClient() *Client {
	client := NewClient(0)
	client.allowAddress = func(netip.Addr) bool { return true }
	return client
}

func TestUpstreamAddressGuardRejectsNonPublicNetworks(t *testing.T) {
	for _, raw := range []string{
		"127.0.0.1", "10.0.0.1", "172.16.0.1", "192.168.0.1",
		"169.254.10.1", "100.64.0.1", "0.0.0.1", "192.0.2.1",
		"198.18.0.1", "203.0.113.1", "224.0.0.1", "255.255.255.255",
		"::1", "fc00::1", "fec0::1", "fe80::1", "ff02::1", "2001:db8::1",
		"64:ff9b::7f00:1", "2002:7f00:1::", "::ffff:127.0.0.1",
	} {
		if isPublicUpstreamAddr(netip.MustParseAddr(raw)) {
			t.Errorf("non-public address %s was accepted", raw)
		}
	}
	for _, raw := range []string{"93.184.216.34", "2606:2800:220:1:248:1893:25c8:1946"} {
		if !isPublicUpstreamAddr(netip.MustParseAddr(raw)) {
			t.Errorf("public address %s was rejected", raw)
		}
	}
}

func TestUpstreamClientRejectsLoopbackDestination(t *testing.T) {
	client := NewClient(0)
	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:1/private", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.DiagnosticClient().Do(req); err == nil {
		t.Fatal("loopback upstream destination was accepted")
	}
}

func TestUpstreamDialPinsResolvedAddress(t *testing.T) {
	client := NewClient(0)
	client.lookupNetIP = func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
	}
	var dialed string
	stop := errors.New("stop after address capture")
	_, err := client.dialPublicTarget(
		context.Background(),
		"tcp",
		"upstream.example:443",
		func(_ context.Context, _ string, address string) (net.Conn, error) {
			dialed = address
			return nil, stop
		},
	)
	if !errors.Is(err, stop) {
		t.Fatalf("dial error = %v, want sentinel", err)
	}
	if dialed != "93.184.216.34:443" {
		t.Fatalf("dialed address = %q, want vetted IP", dialed)
	}
}

func TestUpstreamDialRejectsUnsafeDNSAnswerBeforeDial(t *testing.T) {
	client := NewClient(0)
	client.lookupNetIP = func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{
			netip.MustParseAddr("93.184.216.34"),
			netip.MustParseAddr("127.0.0.1"),
		}, nil
	}
	dialCalled := false
	_, err := client.dialPublicTarget(
		context.Background(),
		"tcp",
		"upstream.example:443",
		func(context.Context, string, string) (net.Conn, error) {
			dialCalled = true
			return nil, errors.New("unexpected dial")
		},
	)
	if err == nil {
		t.Fatal("mixed public/private DNS answer was accepted")
	}
	if dialCalled {
		t.Fatal("dial happened before all resolved addresses were validated")
	}
}

func TestUpstreamClientDoesNotFollowRedirects(t *testing.T) {
	var redirected atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirected" {
			redirected.Store(true)
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Redirect(w, r, "/redirected", http.StatusFound)
	}))
	defer server.Close()

	client := newPrivateNetworkTestClient()
	req, err := http.NewRequest(http.MethodGet, server.URL+"/start", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.DiagnosticClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusFound)
	}
	if redirected.Load() {
		t.Fatal("upstream redirect was followed")
	}
}
