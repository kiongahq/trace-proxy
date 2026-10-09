# trace-proxy

Kionga's LLM egress. Agents send their LLM calls here instead of to a provider. The
proxy forwards each call to the configured upstream and publishes a trace of it
(prompt, response, timing, token usage, agent name and version) to Kafka, without
any change to agent code. Provider keys stay in the proxy's environment and never
appear in traces or logs.

It exposes an **OpenAI-compatible** API, so any OpenAI-style client works by
pointing its base URL here. The upstream can be a hosted provider or a self-hosted
OpenAI-compatible server such as vLLM.

Part of [Kionga](https://github.com/kiongahq): see the
[architecture overview](https://github.com/kiongahq#how-the-pieces-connect) and the
[deploy](https://github.com/kiongahq/deploy) repo for running it with everything else.

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `PORT` | `8081` | Listen port |
| `LLM_UPSTREAM_URL` | `http://localhost:8000` | Provider or self-hosted server to forward to |
| `TRACE_SINK_FORMAT` | HTTP | `kafka-rest` to publish through a Kafka REST proxy |
| `TRACE_SINK_URL` | unset | e.g. `http://kafka-rest:8082/topics/mlaiops.llm.traces` |
| `TRACE_SINK_TOKEN` | unset | Bearer token sent to an HTTP trace sink |
| `MLAIOPS_AGENT_NAME` / `MLAIOPS_AGENT_VERSION` | `unknown` | Labels added to every trace |

`GET /healthz` reports liveness.

## Develop

```bash
make verify          # gofmt, vet, race tests, build
make image           # ghcr.io/kiongahq/trace-proxy:dev
```

CI publishes `ghcr.io/kiongahq/trace-proxy` on pushes to `main` (`latest`,
`sha-<short>`) and on `v*` tags.
