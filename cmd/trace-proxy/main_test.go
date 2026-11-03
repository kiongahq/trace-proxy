package main

import (
	"context"
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
