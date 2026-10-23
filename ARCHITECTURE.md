# Architecture

This document describes how the trace proxy is built and how it fits into Kionga.
Read it before you change what the proxy records or forwards.

## Bird's-eye view

The trace proxy is a single-host reverse proxy built on the standard library's
`httputil.ReverseProxy`. Every request except `GET /healthz` is forwarded to one
upstream (`LLM_UPSTREAM_URL`) with its path, query, headers and body intact. After
the upstream responds, the proxy builds a small trace event and hands it to a sink
in a separate goroutine, so publishing never delays or fails the LLM call.

The proxy is path-agnostic: it forwards whatever API the client speaks. Kionga uses
it for OpenAI-compatible calls, which is why the agent runtime's `openai` and
`openai-compatible` backends point their base URL at it.

## Code map

### `cmd/trace-proxy`

`main.go` loads configuration, parses `LLM_UPSTREAM_URL` (default
`http://localhost:8000`), picks the sink from `TRACE_SINK_FORMAT`, and serves on
`PORT` (default `8081`) with 120-second read, write and idle timeouts.

### `internal/traceproxy`

`proxy.go` holds the whole data path:

- `New` wraps the reverse proxy. Its director adds `X-MLAIOps-Agent` and
  `X-MLAIOps-Agent-Version` headers to the upstream request. A `statusRecorder`
  captures the upstream status code.
- `Event` is the trace record: `agent`, `version`, `session_id` (from the client's
  `X-Session-ID` header), `status_code`, `duration_ms`, `created_at`, and an
  optional `model` field that the proxy does not fill in today.
- `Sink` is the publishing interface. `HTTPSink` posts the event as JSON to
  `TRACE_SINK_URL`, with `TRACE_SINK_TOKEN` as a bearer token when set.
  `KafkaRESTSink` posts it in the Kafka REST v2 envelope
  (`{"records": [{"value": ...}]}`, content type
  `application/vnd.kafka.json.v2+json`). Both do nothing when `TRACE_SINK_URL` is
  empty and both ignore delivery errors.

`proxy_test.go` checks forwarding, the agent header and the emitted event against
an in-process upstream and an in-memory sink, and the Kafka REST envelope against
an in-process topic endpoint.

### `internal/runtimeconfig`

Loads non-secret settings from `KIONGA_CONFIG_FILE` and secrets from `<NAME>_FILE`
files, with the same rules as the platform gateway. It started as a copy of the
platform's `go/internal/runtimeconfig`; keep the shared loading rules in step.

### `Dockerfile`, `Makefile`

A two-stage build: a static, stripped binary from `golang:1.26.10-alpine`, run on
`gcr.io/distroless/static-debian12:nonroot`. `make verify`, `make build` and
`make image` are the local entry points.

## Invariants

- **Transparent forwarding.** The proxy does not rewrite request or response bodies
  and does not add or remove credentials. The client's `Authorization` header goes
  upstream as sent, and no credential is copied into an event.
- **Observability never breaks the call.** Events are emitted asynchronously with a
  5-second client timeout; a slow or failing sink cannot fail or delay the
  response.
- **Bodies are never recorded.** An event carries metadata only, so prompts and
  completions do not leave the request path through the proxy.

## How it connects to other components

- **agent-runtime**: in the local Compose stack, `MLAIOPS_LLM_BASE_URL` defaults to
  `http://trace-proxy:8081/v1`, so `openai` and `openai-compatible` agent calls go
  through one shared proxy. The `anthropic` backend calls Anthropic directly and is
  not traced here.
- **platform**: on Kubernetes, the operator adds the proxy as a `trace-proxy`
  sidecar container (port `8081`, image `TRACE_PROXY_IMAGE`) to every agent pod,
  with `MLAIOPS_AGENT_NAME` set to the agent's name.
- **Kafka**: `deploy/compose.yaml` sets `TRACE_SINK_FORMAT=kafka-rest` and
  publishes to the `mlaiops.llm.traces` topic through the `kafka-rest` service.

## Cross-cutting concerns

- **Event schema.** Consumers of `mlaiops.llm.traces` read the `Event` JSON fields.
  Adding a field is compatible; renaming or removing one is not.
- **Security gates.** CI runs gofmt, vet, race tests, build, govulncheck, gosec and
  gitleaks through the shared Go workflow. See [CONTRIBUTING.md](CONTRIBUTING.md).
