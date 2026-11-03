package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kiongahq/trace-proxy/internal/runtimeconfig"
	"github.com/kiongahq/trace-proxy/internal/traceproxy"
)

func main() {
	if err := runtimeconfig.Load(); err != nil {
		log.Fatal(err)
	}
	cfg, err := configure()
	if err != nil {
		log.Fatal(err)
	}
	handler, err := traceproxy.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	server := &http.Server{Addr: ":" + env("PORT", "8081"), Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 120 * time.Second, WriteTimeout: 120 * time.Second, IdleTimeout: 120 * time.Second}
	mode := "project keys required, no upstream credential"
	if cfg.UpstreamCredential.Value != "" {
		mode = "project keys required, upstream credential injected as " + cfg.UpstreamCredential.Header
	}
	if cfg.AllowUnauthenticated {
		mode = "UNAUTHENTICATED (TRACE_PROXY_ALLOW_UNAUTHENTICATED=true; development only)"
		log.Printf("WARNING: trace proxy accepts requests without a project key and forwards client credentials")
	}
	log.Printf("trace proxy listening on %s and forwarding to %s; %s", server.Addr, cfg.Upstream.Redacted(), mode)
	log.Fatal(server.ListenAndServe())
}

// configure builds the proxy configuration from the environment.
func configure() (traceproxy.Config, error) {
	upstream, err := url.Parse(env("LLM_UPSTREAM_URL", "http://localhost:8000"))
	if err != nil {
		return traceproxy.Config{}, errors.New("LLM_UPSTREAM_URL is not a valid URL")
	}
	client := &http.Client{Timeout: 5 * time.Second}
	var sink traceproxy.Sink = traceproxy.HTTPSink{URL: os.Getenv("TRACE_SINK_URL"), Token: os.Getenv("TRACE_SINK_TOKEN"), Client: client}
	if os.Getenv("TRACE_SINK_FORMAT") == "kafka-rest" {
		sink = traceproxy.KafkaRESTSink{URL: os.Getenv("TRACE_SINK_URL"), Client: client}
	}
	cfg := traceproxy.Config{Upstream: upstream, Agent: env("MLAIOPS_AGENT_NAME", "unknown"), Version: env("MLAIOPS_AGENT_VERSION", "unknown"), Sink: sink}
	if raw := strings.TrimSpace(os.Getenv("TRACE_PROXY_KEYS")); raw != "" {
		if cfg.Keys, err = traceproxy.ParseKeys(raw); err != nil {
			return traceproxy.Config{}, err
		}
	}
	if value := strings.TrimSpace(os.Getenv("TRACE_PROXY_ALLOW_UNAUTHENTICATED")); value != "" {
		allow, err := strconv.ParseBool(value)
		if err != nil {
			return traceproxy.Config{}, errors.New("TRACE_PROXY_ALLOW_UNAUTHENTICATED must be true or false")
		}
		cfg.AllowUnauthenticated = allow
	}
	if cfg.UpstreamCredential, err = traceproxy.NewUpstreamCredential(os.Getenv("LLM_UPSTREAM_API_KEY"), os.Getenv("LLM_UPSTREAM_AUTH_HEADER"), os.Getenv("LLM_UPSTREAM_AUTH_SCHEME")); err != nil {
		return traceproxy.Config{}, err
	}
	if cfg.AllowUnauthenticated && os.Getenv("KIONGA_ENVIRONMENT") == "production" {
		return traceproxy.Config{}, fmt.Errorf("production requires TRACE_PROXY_KEYS; TRACE_PROXY_ALLOW_UNAUTHENTICATED is for local development only")
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
