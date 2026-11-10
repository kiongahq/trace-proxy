package traceproxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

const (
	supportKey = "kionga-test-support-key-0123456789"
	otherKey   = "kionga-test-other-key-000000000000"
)

func testKeys(t *testing.T) *KeyStore {
	t.Helper()
	raw, _ := json.Marshal([]map[string]string{
		{"key_id": "support-prod", "key_sha256": HashKey(supportKey), "project": "customer-care", "agent": "support-bot"},
		{"key_id": "other", "key_sha256": strings.ToUpper(HashKey(otherKey)), "project": "fraud"},
	})
	keys, err := ParseKeys(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	return keys
}

type upstreamRecorder struct {
	hits    atomic.Int32
	headers chan http.Header
}

func newUpstream(t *testing.T) (*upstreamRecorder, *url.URL) {
	t.Helper()
	recorder := &upstreamRecorder{headers: make(chan http.Header, 4)}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder.hits.Add(1)
		recorder.headers <- r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	target, _ := url.Parse(server.URL)
	return recorder, target
}

func securedProxy(t *testing.T, cfg Config) *httptest.Server {
	t.Helper()
	if cfg.Keys == nil {
		cfg.Keys = testKeys(t)
	}
	cfg.Agent, cfg.Version = "shared", "1"
	handler, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

func call(t *testing.T, server *httptest.Server, headers map[string]string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", strings.NewReader(`{}`))
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return resp, string(body)
}

func TestNewRequiresKeysOrExplicitOptIn(t *testing.T) {
	target, _ := url.Parse("http://upstream")
	if _, err := New(Config{Upstream: target}); err == nil || !strings.Contains(err.Error(), "TRACE_PROXY_KEYS") {
		t.Fatalf("a proxy without inbound keys must not start: %v", err)
	}
	if _, err := New(Config{Upstream: target, Keys: testKeys(t), AllowUnauthenticated: true}); err == nil {
		t.Fatal("keys and the unauthenticated opt-in are contradictory")
	}
	if _, err := New(Config{Upstream: &url.URL{Path: "relative"}, AllowUnauthenticated: true}); err == nil {
		t.Fatal("a relative upstream must be rejected")
	}
}

func TestMissingOrUnknownKeyIsRejectedBeforeUpstream(t *testing.T) {
	upstream, target := newUpstream(t)
	events := make(channelSink, 4)
	server := securedProxy(t, Config{Upstream: target, Sink: events})
	for name, headers := range map[string]map[string]string{
		"no key":          {},
		"unknown bearer":  {"Authorization": "Bearer sk-not-a-kionga-key"},
		"unknown x-api":   {"X-Api-Key": "sk-not-a-kionga-key"},
		"basic scheme":    {"Authorization": "Basic " + supportKey},
		"bare key":        {"Authorization": supportKey},
		"empty bearer":    {"Authorization": "Bearer "},
		"key with suffix": {"Authorization": "Bearer " + supportKey + "x"},
	} {
		t.Run(name, func(t *testing.T) {
			resp, body := call(t, server, headers)
			var payload struct {
				Error struct {
					Type string `json:"type"`
				} `json:"error"`
			}
			if resp.StatusCode != http.StatusUnauthorized || json.Unmarshal([]byte(body), &payload) != nil || payload.Error.Type != "invalid_api_key" {
				t.Fatalf("want an OpenAI-shaped 401, got %d %s", resp.StatusCode, body)
			}
		})
	}
	if upstream.hits.Load() != 0 {
		t.Fatalf("rejected requests reached the upstream %d times", upstream.hits.Load())
	}
	if len(events) != 0 {
		t.Fatal("rejected requests must not emit traces")
	}
}

func TestValidKeyProxiesAndAttributesTrace(t *testing.T) {
	_, target := newUpstream(t)
	events := make(channelSink, 4)
	server := securedProxy(t, Config{Upstream: target, Sink: events})
	for _, tc := range []struct {
		headers               map[string]string
		project, keyID, agent string
	}{
		{map[string]string{"Authorization": "Bearer " + supportKey, "X-Session-ID": "s-1"}, "customer-care", "support-prod", "support-bot"},
		{map[string]string{"X-Api-Key": otherKey}, "fraud", "other", "shared"},
		{map[string]string{"Api-Key": otherKey}, "fraud", "other", "shared"},
	} {
		resp, body := call(t, server, tc.headers)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("valid key rejected: %d %s", resp.StatusCode, body)
		}
		event := <-events
		if event.Project != tc.project || event.KeyID != tc.keyID || event.Agent != tc.agent {
			t.Fatalf("trace not attributed to the key: %#v", event)
		}
		raw, _ := json.Marshal(event)
		if strings.Contains(string(raw), supportKey) || strings.Contains(string(raw), otherKey) || strings.Contains(string(raw), HashKey(supportKey)) {
			t.Fatalf("the key leaked into the trace: %s", raw)
		}
	}
}

func TestProjectKeyNeverReachesUpstream(t *testing.T) {
	upstream, target := newUpstream(t)
	server := securedProxy(t, Config{Upstream: target, Sink: make(channelSink, 4)})
	call(t, server, map[string]string{"Authorization": "Bearer " + supportKey, "X-Api-Key": "extra", "Api-Key": "extra"})
	headers := <-upstream.headers
	for _, name := range clientCredentialHeaders {
		if headers.Get(name) != "" {
			t.Fatalf("client credential header %s reached the upstream: %q", name, headers.Get(name))
		}
	}
	if headers.Get("X-Kionga-Agent") != "support-bot" {
		t.Fatalf("agent header should follow the key's agent: %v", headers)
	}
}

func TestHealthzStaysOpen(t *testing.T) {
	_, target := newUpstream(t)
	server := securedProxy(t, Config{Upstream: target})
	resp, err := http.Get(server.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz must not need a key: %d", resp.StatusCode)
	}
}

func TestParseKeysRejectsBadInput(t *testing.T) {
	good := HashKey(supportKey)
	for name, raw := range map[string]string{
		"not json":        `nope`,
		"empty":           `[]`,
		"plaintext field": `[{"key_id":"a","key":"secret","key_sha256":"` + good + `","project":"p"}]`,
		"short hash":      `[{"key_id":"a","key_sha256":"abc","project":"p"}]`,
		"no project":      `[{"key_id":"a","key_sha256":"` + good + `"}]`,
		"bad id":          `[{"key_id":"a b","key_sha256":"` + good + `","project":"p"}]`,
		"duplicate id":    `[{"key_id":"a","key_sha256":"` + good + `","project":"p"},{"key_id":"a","key_sha256":"` + HashKey(otherKey) + `","project":"p"}]`,
		"duplicate hash":  `[{"key_id":"a","key_sha256":"` + good + `","project":"p"},{"key_id":"b","key_sha256":"` + good + `","project":"p"}]`,
		"allowed_models":  `[{"key_id":"a","key_sha256":"` + good + `","project":"p","allowed_models":["x"]}]`,
	} {
		if _, err := ParseKeys(raw); err == nil {
			t.Errorf("%s: ParseKeys accepted %s", name, raw)
		} else if strings.Contains(err.Error(), "secret") {
			t.Errorf("%s: error echoes input: %v", name, err)
		}
	}
}
