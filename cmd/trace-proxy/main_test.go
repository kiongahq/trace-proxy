package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/kiongahq/trace-proxy/internal/runtimeconfig"
	"github.com/kiongahq/trace-proxy/internal/traceproxy"
)

func clearProxyEnv(t *testing.T) {
	for _, key := range []string{"TRACE_PROXY_KEYS", "TRACE_PROXY_KEYS_FILE", "TRACE_PROXY_ALLOW_UNAUTHENTICATED", "KIONGA_ENVIRONMENT", "LLM_UPSTREAM_API_KEY", "LLM_UPSTREAM_API_KEY_FILE", "LLM_UPSTREAM_AUTH_HEADER", "LLM_UPSTREAM_AUTH_SCHEME", "KIONGA_CONFIG_FILE"} {
		t.Setenv(key, "")
	}
}

func TestConfigureReadsKeys(t *testing.T) {
	clearProxyEnv(t)
	t.Setenv("TRACE_PROXY_KEYS", `[{"key_id":"a","key_sha256":"`+traceproxy.HashKey("k")+`","project":"p"}]`)
	cfg, err := configure()
	if err != nil || cfg.Keys == nil || cfg.AllowUnauthenticated {
		t.Fatalf("unexpected config %+v %v", cfg, err)
	}
	t.Setenv("TRACE_PROXY_KEYS", `not json`)
	if _, err := configure(); err == nil {
		t.Fatal("invalid keys must be rejected")
	}
}

func TestConfigureUnauthenticatedOptIn(t *testing.T) {
	clearProxyEnv(t)
	t.Setenv("TRACE_PROXY_ALLOW_UNAUTHENTICATED", "true")
	if cfg, err := configure(); err != nil || !cfg.AllowUnauthenticated {
		t.Fatalf("explicit development opt-in failed: %+v %v", cfg, err)
	}
	t.Setenv("TRACE_PROXY_ALLOW_UNAUTHENTICATED", "yes please")
	if _, err := configure(); err == nil {
		t.Fatal("an invalid boolean must be rejected")
	}
	t.Setenv("TRACE_PROXY_ALLOW_UNAUTHENTICATED", "true")
	t.Setenv("KIONGA_ENVIRONMENT", "production")
	if _, err := configure(); err == nil {
		t.Fatal("production must refuse the unauthenticated mode")
	}
}

// TestMainRefusesToStartWithoutKeys runs main in a child process: with no
// keys and no opt-in it must exit non-zero before it listens.
func TestMainRefusesToStartWithoutKeys(t *testing.T) {
	if os.Getenv("TRACE_PROXY_MAIN_CHILD") == "1" {
		main()
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestMainRefusesToStartWithoutKeys$") // #nosec G204 G702 -- re-executes this test binary
	cmd.Env = append(os.Environ(), "TRACE_PROXY_MAIN_CHILD=1", "TRACE_PROXY_KEYS=", "TRACE_PROXY_KEYS_FILE=", "TRACE_PROXY_ALLOW_UNAUTHENTICATED=", "KIONGA_CONFIG_FILE=", "PORT=0")
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("trace-proxy kept running without keys: %s", output)
	}
	if err == nil || !strings.Contains(string(output), "requires TRACE_PROXY_KEYS") {
		t.Fatalf("expected a clear startup failure, got err=%v output=%s", err, output)
	}
}

func TestKeysLoadFromMountedFile(t *testing.T) {
	clearProxyEnv(t)
	file := t.TempDir() + "/keys.json"
	if err := os.WriteFile(file, []byte(`[{"key_id":"a","key_sha256":"`+traceproxy.HashKey("k")+`","project":"p"}]`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TRACE_PROXY_KEYS_FILE", file)
	if err := runtimeconfig.Load(); err != nil {
		t.Fatal(err)
	}
	if cfg, err := configure(); err != nil || cfg.Keys == nil {
		t.Fatalf("TRACE_PROXY_KEYS_FILE was not honoured: %v", err)
	}
}

func TestConfigureUpstreamCredentialFromFile(t *testing.T) {
	clearProxyEnv(t)
	file := t.TempDir() + "/upstream-key"
	if err := os.WriteFile(file, []byte("sk-from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LLM_UPSTREAM_API_KEY_FILE", file)
	t.Setenv("LLM_UPSTREAM_AUTH_HEADER", "x-api-key")
	t.Setenv("TRACE_PROXY_KEYS", `[{"key_id":"a","key_sha256":"`+traceproxy.HashKey("k")+`","project":"p"}]`)
	if err := runtimeconfig.Load(); err != nil {
		t.Fatal(err)
	}
	cfg, err := configure()
	if err != nil || cfg.UpstreamCredential.Header != "X-Api-Key" || cfg.UpstreamCredential.Value != "sk-from-file" {
		t.Fatalf("upstream credential not loaded from file: %+v %v", cfg.UpstreamCredential, err)
	}
	t.Setenv("LLM_UPSTREAM_AUTH_HEADER", "not a header")
	if _, err := configure(); err == nil {
		t.Fatal("an invalid header name must be rejected")
	}
}

func TestWriteTimeoutFromEnv(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  time.Duration
		ok    bool
	}{
		{"", 660 * time.Second, true},
		{"  ", 660 * time.Second, true},
		{"1", time.Second, true},
		{"900", 900 * time.Second, true},
		{"3600", time.Hour, true},
		{"3601", 0, false},
		{"0", 0, false},
		{"-5", 0, false},
		{"soon", 0, false},
		{"1.5", 0, false},
		{"60s", 0, false},
		{"99999999999999999999", 0, false},
	} {
		t.Setenv("TRACE_PROXY_WRITE_TIMEOUT_SECONDS", tc.value)
		got, err := writeTimeoutFromEnv()
		if (err == nil) != tc.ok || got != tc.want {
			t.Errorf("value %q: got %s, err %v; want %s, ok=%v", tc.value, got, err, tc.want, tc.ok)
		}
	}
}

// The proxy must outlast the callers: runner 600 s < gateway about 630 s < proxy.
func TestDefaultWriteTimeoutOutlastsCallers(t *testing.T) {
	if defaultWriteTimeout <= 630*time.Second {
		t.Fatalf("default write timeout %s does not outlast the gateway's 630 s", defaultWriteTimeout)
	}
	server := newServer(":0", http.NotFoundHandler(), defaultWriteTimeout)
	if server.WriteTimeout != defaultWriteTimeout {
		t.Fatalf("server write timeout = %s", server.WriteTimeout)
	}
}

func TestMainRefusesAnInvalidWriteTimeout(t *testing.T) {
	if os.Getenv("TRACE_PROXY_TIMEOUT_CHILD") == "1" {
		main()
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestMainRefusesAnInvalidWriteTimeout$") // #nosec G204 G702 -- re-executes this test binary
	cmd.Env = append(os.Environ(), "TRACE_PROXY_TIMEOUT_CHILD=1", "TRACE_PROXY_WRITE_TIMEOUT_SECONDS=0", "TRACE_PROXY_ALLOW_UNAUTHENTICATED=true", "TRACE_PROXY_KEYS=", "TRACE_PROXY_KEYS_FILE=", "KIONGA_CONFIG_FILE=", "LLM_UPSTREAM_API_KEY=", "LLM_UPSTREAM_API_KEY_FILE=", "KIONGA_ENVIRONMENT=", "PORT=0")
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("trace-proxy kept running with an invalid timeout: %s", output)
	}
	if err == nil || !strings.Contains(string(output), "TRACE_PROXY_WRITE_TIMEOUT_SECONDS") {
		t.Fatalf("expected a clear startup failure, got err=%v output=%s", err, output)
	}
}

// slowProxyCall sends a POST with a body through a real server built by newServer
// to an upstream that replies after delay, and returns the reply or the read error.
func slowProxyCall(t *testing.T, writeTimeout, delay time.Duration) (int, string, error) {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		time.Sleep(delay)
		_, _ = w.Write([]byte("completion"))
	}))
	defer upstream.Close()
	target, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := traceproxy.New(traceproxy.Config{Upstream: target, AllowUnauthenticated: true, Sink: traceproxy.HTTPSink{}})
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := newServer("", handler, writeTimeout)
	go func() { _ = server.Serve(listener) }()
	defer func() { _ = server.Close() }()
	response, err := http.Post("http://"+listener.Addr().String()+"/v1/chat/completions", "application/json", strings.NewReader(`{"model":"m"}`))
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	return response.StatusCode, string(body), err
}

// A slow response inside the configured timeout arrives; one past it does not, which
// proves this setting, not a fixed limit, is what governs it.
func TestConfiguredWriteTimeoutGovernsSlowResponses(t *testing.T) {
	status, body, err := slowProxyCall(t, 1500*time.Millisecond, 400*time.Millisecond)
	if err != nil || status != http.StatusOK || body != "completion" {
		t.Fatalf("a slow response inside the timeout was lost: %d %q %v", status, body, err)
	}
	if status, body, err = slowProxyCall(t, 200*time.Millisecond, 500*time.Millisecond); err == nil {
		t.Fatalf("a response past the configured timeout should be cut off, got %d %q", status, body)
	}
}
