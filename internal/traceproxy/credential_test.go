package traceproxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const providerKey = "sk-provider-held-by-the-proxy"

func TestUpstreamKeyReplacesClientCredentials(t *testing.T) {
	upstream, target := newUpstream(t)
	credential, err := NewUpstreamCredential(providerKey, "", "")
	if err != nil {
		t.Fatal(err)
	}
	server := securedProxy(t, Config{Upstream: target, Sink: make(channelSink, 4), UpstreamCredential: credential})
	call(t, server, map[string]string{"Authorization": "Bearer " + supportKey, "X-Api-Key": "client-secret", "Api-Key": "client-secret", "Proxy-Authorization": "Basic client-secret"})
	headers := <-upstream.headers
	if got := headers.Get("Authorization"); got != "Bearer "+providerKey {
		t.Fatalf("upstream Authorization = %q, want the proxy's provider key", got)
	}
	for name, values := range headers {
		for _, value := range values {
			if strings.Contains(value, "client-secret") || strings.Contains(value, supportKey) {
				t.Fatalf("client credential reached the upstream in %s: %q", name, value)
			}
		}
	}
}

func TestUpstreamKeyHeaderIsConfigurable(t *testing.T) {
	for _, tc := range []struct {
		header, scheme, wantHeader, wantValue string
	}{
		{"x-api-key", "", "X-Api-Key", providerKey},
		{"api-key", "", "Api-Key", providerKey},
		{"Authorization", "Token", "Authorization", "Token " + providerKey},
	} {
		upstream, target := newUpstream(t)
		credential, err := NewUpstreamCredential(providerKey, tc.header, tc.scheme)
		if err != nil {
			t.Fatal(err)
		}
		server := securedProxy(t, Config{Upstream: target, Sink: make(channelSink, 4), UpstreamCredential: credential})
		call(t, server, map[string]string{"X-Api-Key": supportKey, "Authorization": ""})
		headers := <-upstream.headers
		if headers.Get(tc.wantHeader) != tc.wantValue {
			t.Fatalf("%s: upstream %s = %q, want %q", tc.header, tc.wantHeader, headers.Get(tc.wantHeader), tc.wantValue)
		}
		if tc.wantHeader != "Authorization" && headers.Get("Authorization") != "" {
			t.Fatalf("%s: unexpected Authorization upstream", tc.header)
		}
	}
}

// Without LLM_UPSTREAM_API_KEY an authenticated proxy sends no credential at
// all (self-hosted upstreams); the caller's key is never passed through.
func TestNoUpstreamKeyMeansNoCredentialUpstream(t *testing.T) {
	upstream, target := newUpstream(t)
	server := securedProxy(t, Config{Upstream: target, Sink: make(channelSink, 4)})
	call(t, server, map[string]string{"Authorization": "Bearer " + supportKey})
	headers := <-upstream.headers
	for _, name := range clientCredentialHeaders {
		if headers.Get(name) != "" {
			t.Fatalf("%s must be absent upstream without LLM_UPSTREAM_API_KEY, got %q", name, headers.Get(name))
		}
	}
}

// Passthrough of client credentials exists only in the explicit development
// mode, which cannot be combined with an injected provider key.
func TestUnauthenticatedModePassesClientCredentialsAndRefusesProviderKey(t *testing.T) {
	upstream, target := newUpstream(t)
	handler, err := New(Config{Upstream: target, Sink: make(channelSink, 4), AllowUnauthenticated: true})
	if err != nil {
		t.Fatal(err)
	}
	server := newServer(t, handler)
	call(t, server, map[string]string{"Authorization": "Bearer client-own-key"})
	if got := (<-upstream.headers).Get("Authorization"); got != "Bearer client-own-key" {
		t.Fatalf("development mode should forward the client's credential, got %q", got)
	}
	credential, _ := NewUpstreamCredential(providerKey, "", "")
	if _, err := New(Config{Upstream: target, AllowUnauthenticated: true, UpstreamCredential: credential}); err == nil || !strings.Contains(err.Error(), "TRACE_PROXY_KEYS") {
		t.Fatalf("an injected key without inbound auth is an open relay and must be refused: %v", err)
	}
}

func TestNewUpstreamCredentialValidates(t *testing.T) {
	if credential, err := NewUpstreamCredential("", "x-api-key", ""); err != nil || credential.Value != "" {
		t.Fatalf("no key means no credential: %+v %v", credential, err)
	}
	for _, tc := range [][3]string{
		{"key", "bad header", ""},
		{"key\r\nX-Evil: 1", "", ""},
		{"key", "", "Bearer extra"},
	} {
		if _, err := NewUpstreamCredential(tc[0], tc[1], tc[2]); err == nil {
			t.Errorf("NewUpstreamCredential(%q) accepted an invalid value", tc)
		} else if strings.Contains(err.Error(), "key\r") {
			t.Errorf("error echoes the key: %v", err)
		}
	}
}

func newServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}
