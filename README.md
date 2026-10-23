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
change: you point its base URL at the proxy.

Each event records the agent name and version, the session ID, the upstream HTTP
status, the duration and a timestamp. Request and response bodies are not recorded.

## Install

The image is published as `ghcr.io/kiongahq/trace-proxy`. To build from source you
need Go 1.26:

```bash
git clone git@github.com:kiongahq/trace-proxy.git
cd trace-proxy
make build          # bin/trace-proxy
```

## Usage

```bash
LLM_UPSTREAM_URL=https://api.openai.com \
TRACE_SINK_URL=http://localhost:9999/traces \
MLAIOPS_AGENT_NAME=support-bot \
./bin/trace-proxy                                  # listens on :8081

curl http://localhost:8081/v1/chat/completions \
  -H "Authorization: Bearer $OPENAI_API_KEY" \
  -H 'X-Session-ID: demo-1' \
  -H 'Content-Type: application/json' \
  -d '{"model": "gpt-4o-mini", "messages": [{"role": "user", "content": "Hello"}]}'
```

The client's own `Authorization` header goes to the upstream as sent. Set
`TRACE_SINK_FORMAT=kafka-rest` and a Kafka REST topic URL (for example
`http://kafka-rest:8082/topics/mlaiops.llm.traces`) to publish to Kafka.
`GET /healthz` reports liveness. The
[Configuration reference](https://kiongahq.github.io/docs/reference/configuration/)
lists every setting.

## Documentation

- Product documentation: [Build agents](https://kiongahq.github.io/docs/guides/agents/)
- Engineering handbook: [AI and observability](https://kiongahq.github.io/docs/engineering/components/ai-observability/)
- How this repository is laid out: [ARCHITECTURE.md](ARCHITECTURE.md)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for the development setup, the checks CI
runs and the commit conventions.

## License

[MIT](LICENCE) © Kionga contributors
