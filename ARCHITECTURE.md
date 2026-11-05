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
`http://localhost:8000`), the project keys (`TRACE_PROXY_KEYS`) and the upstream
credential (`LLM_UPSTREAM_API_KEY`, `LLM_UPSTREAM_AUTH_HEADER`,
`LLM_UPSTREAM_AUTH_SCHEME`), picks the sink from `TRACE_SINK_FORMAT`, and serves on
`PORT` (default `8081`) with 120-second read and idle timeouts and a write timeout
of `TRACE_PROXY_WRITE_TIMEOUT_SECONDS` (default `660`, from 1 to `3600`, anything else
stops startup). The write timeout covers a whole response, stream included, and must
outlast every caller: agent runner 600 s < gateway client about 630 s < proxy 660 s.
If the proxy gives up first, the provider call completes and is billed, the reply is
lost and the caller retries. The read timeout does not end slow responses, because
`net/http` clears the read deadline once the request body is read, and the reverse
proxy's default transport has no response header timeout.

### `internal/traceproxy`

`proxy.go` holds the whole data path:

- `New(Config)` wraps the reverse proxy. It refuses to build without inbound
  keys unless `AllowUnauthenticated` is set. The handler authenticates every
  request except `/healthz` against the `KeyStore` (`keys.go`) before proxying and
  answers an unknown or missing key with an OpenAI-shaped `401`
  (`{"error":{"type":"invalid_api_key",...}}`); the upstream never sees it. The
  rewrite step removes the caller's credential headers and adds
  `X-MLAIOps-Agent` and `X-MLAIOps-Agent-Version`. A `statusRecorder` captures
  the upstream status code; it implements `Flush` and `Unwrap`, so the reverse
  proxy flushes each `text/event-stream` chunk at once and streamed completions
  arrive token by token.
- `KeyStore` holds only SHA-256 hashes of project keys, parsed from
  `TRACE_PROXY_KEYS`. It accepts `Authorization: Bearer <key>`, `x-api-key` or
  `api-key` and compares the hash with every stored hash in constant time.
- `Event` is the trace record: `agent` (the key's agent, or
  `MLAIOPS_AGENT_NAME`), `version`, `project` and `key_id` (from the matched key;
  the key itself is never recorded), `session_id` (from the client's
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

- **Authenticated, credential-isolating forwarding.** The proxy does not rewrite
  request or response bodies. Every request except `/healthz` needs a project
  key. The caller's credential headers never reach the upstream; the proxy sends
  its own `LLM_UPSTREAM_API_KEY` (header `LLM_UPSTREAM_AUTH_HEADER`) or nothing.
  No key or credential is copied into an event or a log. The only passthrough is
  the explicit development mode `TRACE_PROXY_ALLOW_UNAUTHENTICATED=true`, which
  production and an injected provider key both refuse.
- **Observability never breaks the call.** Events are emitted asynchronously with a
  5-second client timeout; a slow or failing sink cannot fail or delay the
  response.
- **Bodies are never recorded.** An event carries metadata only, so prompts and
  completions do not leave the request path through the proxy.

## How it connects to other components

- **agent-runtime**: in the local Compose stack, `MLAIOPS_LLM_BASE_URL` defaults to
  `http://trace-proxy:8081/v1`, so `openai` and `openai-compatible` agent calls go
  through one shared proxy. The agent's `OPENAI_API_KEY` is then its Kionga
  project key; the provider key belongs in the proxy's `LLM_UPSTREAM_API_KEY`. The `anthropic` backend calls Anthropic directly and is
  not traced here.
- **platform**: on Kubernetes, the operator adds the proxy as a `trace-proxy`
  sidecar container (port `8081`, image `TRACE_PROXY_IMAGE`) to every agent pod,
  with `MLAIOPS_AGENT_NAME` set to the agent's name. The sidecar needs
  `TRACE_PROXY_KEYS` (or `TRACE_PROXY_KEYS_FILE`) and, for hosted providers,
  `LLM_UPSTREAM_API_KEY`.
- **Kafka**: `deploy/compose.yaml` sets `TRACE_SINK_FORMAT=kafka-rest` and
  publishes to the `mlaiops.llm.traces` topic through the `kafka-rest` service.

## Cross-cutting concerns

- **Event schema.** Consumers of `mlaiops.llm.traces` read the `Event` JSON fields.
  Adding a field is compatible; renaming or removing one is not.
- **Security gates.** CI runs gofmt, vet, race tests, build, govulncheck, gosec and
  gitleaks through the shared Go workflow. See [CONTRIBUTING.md](CONTRIBUTING.md).
