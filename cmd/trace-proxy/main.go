package main

import (
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/mlaiops/platform/internal/traceproxy"
)

func main() {
	raw := env("LLM_UPSTREAM_URL", "http://localhost:8000")
	upstream, err := url.Parse(raw)
	if err != nil {
		log.Fatal(err)
	}
	sink := traceproxy.HTTPSink{URL: os.Getenv("TRACE_SINK_URL"), Token: os.Getenv("TRACE_SINK_TOKEN"), Client: &http.Client{Timeout: 5 * time.Second}}
	handler := traceproxy.New(upstream, env("MLAIOPS_AGENT_NAME", "unknown"), env("MLAIOPS_AGENT_VERSION", "unknown"), sink)
	server := &http.Server{Addr: ":" + env("PORT", "8081"), Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 120 * time.Second, WriteTimeout: 120 * time.Second, IdleTimeout: 120 * time.Second}
	log.Printf("trace proxy listening on %s and forwarding to %s", server.Addr, upstream)
	log.Fatal(server.ListenAndServe())
}
func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
