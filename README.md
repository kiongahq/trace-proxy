# trace-proxy

[![CI](https://github.com/kiongahq/trace-proxy/actions/workflows/ci.yml/badge.svg)](https://github.com/kiongahq/trace-proxy/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENCE)

The Kionga LLM egress proxy: forwards agent LLM calls and records a trace event for each one.

## Background

Kionga is a self-hosted platform for machine learning, data and AI agents. Agents
that use an OpenAI-compatible backend send their LLM calls to this proxy instead of
straight to the provider. The proxy forwards each request unchanged to the
configured upstream, a hosted provider or a self-hosted server such as vLLM, and
publishes one event per call to Kafka or an HTTP endpoint. Agent code needs no
change: you point its base URL at the proxy and use a Kionga project key as its API
key, while the provider key stays in the proxy.

Each event records the agent name and version, the project and key ID of the
caller, the session ID, the upstream HTTP status, the duration and a timestamp. Request and response bodies are not recorded.

## Install

The image is published as `ghcr.io/kiongahq/trace-proxy`. To build from source you
need Go 1.26:

```bash
git clone git@github.com:kiongahq/trace-proxy.git
cd trace-proxy
make build          # bin/trace-proxy
```

## Usage

Every request except `GET /healthz` must carry a Kionga project key. Generate a
key, store only its SHA-256 in `TRACE_PROXY_KEYS` (or a file named by
`TRACE_PROXY_KEYS_FILE`), and give the key itself to the agent:

```bash
KEY=kpk_$(openssl rand -hex 24)
HASH=$(printf %s "$KEY" | shasum -a 256 | cut -d' ' -f1)
export TRACE_PROXY_KEYS='[{"key_id":"support-prod","key_sha256":"'$HASH'","project":"customer-care","agent":"support-bot"}]'

LLM_UPSTREAM_URL=https://api.openai.com \
TRACE_SINK_URL=http://localhost:9999/traces \
./bin/trace-proxy                                  # listens on :8081

curl http://localhost:8081/v1/chat/completions \
  -H "Authorization: Bearer $KEY" \
  -H 'X-Session-ID: demo-1' \
  -H 'Content-Type: application/json' \
  -d '{"model": "gpt-4o-mini", "messages": [{"role": "user", "content": "Hello"}]}'
```

Agents send the key the way their SDK sends a provider key:
`Authorization: Bearer <key>` (OpenAI clients), `x-api-key` (Anthropic clients) or
`api-key` (Azure clients). Point the SDK's API key setting at the project key. A
missing or unknown key gets `401` with
`{"error":{"type":"invalid_api_key",...}}` and never reaches the upstream. Each
entry in `TRACE_PROXY_KEYS` has `key_id` (recorded in traces), `key_sha256`
(lowercase hex), `project` (recorded in traces) and an optional `agent` that
overrides `MLAIOPS_AGENT_NAME` for that key. The proxy refuses to start without
keys. For local development only, `TRACE_PROXY_ALLOW_UNAUTHENTICATED=true` runs it
without keys and forwards the client's own credentials unchanged; production
(`KIONGA_ENVIRONMENT=production`) refuses that mode, and it cannot be combined
with `LLM_UPSTREAM_API_KEY`.

The provider key stays in the proxy. With keys configured, the proxy removes the
caller's `Authorization`, `x-api-key`, `api-key` and `Proxy-Authorization` headers
and, when `LLM_UPSTREAM_API_KEY` is set, adds the provider key itself. Without
`LLM_UPSTREAM_API_KEY` no credential goes upstream, which suits a self-hosted
server such as vLLM. Neither key is written to a trace or a log.

| Variable | Default | Purpose |
| --- | --- | --- |
| `TRACE_PROXY_KEYS` / `TRACE_PROXY_KEYS_FILE` | required | JSON array of hashed project keys |
| `TRACE_PROXY_ALLOW_UNAUTHENTICATED` | `false` | Development only: no keys, client credentials pass through |
| `LLM_UPSTREAM_URL` | `http://localhost:8000` | Upstream origin or `/v1` base |
| `LLM_UPSTREAM_API_KEY` / `LLM_UPSTREAM_API_KEY_FILE` | unset | Provider key injected on every authenticated call |
| `LLM_UPSTREAM_AUTH_HEADER` | `Authorization` | `x-api-key` for Anthropic, `api-key` for Azure OpenAI |
| `LLM_UPSTREAM_AUTH_SCHEME` | `Bearer` for `Authorization`, else none | Prefix before the key |
| `TRACE_SINK_URL`, `TRACE_SINK_FORMAT`, `TRACE_SINK_TOKEN` | unset | Where events go |
| `MLAIOPS_AGENT_NAME`, `MLAIOPS_AGENT_VERSION` | `unknown` | Default agent identity for events |

Set
`TRACE_SINK_FORMAT=kafka-rest` and a Kafka REST topic URL (for example
`http://kafka-rest:8082/topics/mlaiops.llm.traces`) to publish to Kafka.
`GET /healthz` reports liveness. The
[Configuration reference](https://kiongahq.github.io/docs/reference/configuration/)
lists every setting.

`LLM_UPSTREAM_URL` may be an origin, such as `http://localhost:8000` for vLLM
or `https://api.openai.com` for OpenAI. A base ending in `/v1`, such as
`https://api.openai.com/v1`, is also supported: requests already starting with
`/v1` are forwarded with that segment only once. Other paths are joined to the
configured base normally.

## Documentation

- Product documentation: [Build agents](https://kiongahq.github.io/docs/guides/agents/)
- Engineering handbook: [AI and observability](https://kiongahq.github.io/docs/engineering/components/ai-observability/)
- How this repository is laid out: [ARCHITECTURE.md](ARCHITECTURE.md)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for the development setup, the checks CI
runs and the commit conventions.

## License

[MIT](LICENCE) © Kionga contributors
