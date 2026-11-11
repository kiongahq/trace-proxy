# Changelog

## 0.1.0 (2026-11-11)


### ⚠ BREAKING CHANGES

* environment variables, headers and resource names are renamed.

### Features

* **agents:** real agentic AI layer — LangGraph runtime, SDK primitives, gateway invoke path ([4c66ca7](https://github.com/kiongahq/trace-proxy/commit/4c66ca742a03fd1f6e71fd541efcd505236c9dcd))
* **auth:** require login for console access ([098b070](https://github.com/kiongahq/trace-proxy/commit/098b07070bc71cd2049f7accede251f25792ff31))
* coherent console, versioned pipeline DAGs and scheduling ([6239bed](https://github.com/kiongahq/trace-proxy/commit/6239bedda791db4cfdecc5a064532336845b80a3))
* complete Kionga platform foundation ([6c60241](https://github.com/kiongahq/trace-proxy/commit/6c60241ced8d3d0fe86c90fddbf5e1cf3d75425c))
* **data-plane:** real feature store, object browsing, Langfuse, live catalog ([ea243d4](https://github.com/kiongahq/trace-proxy/commit/ea243d436140ce68c490ca384027a79bc3d0b6fb))
* **deploy:** add managed production packages and isolated workspaces ([cc4cd31](https://github.com/kiongahq/trace-proxy/commit/cc4cd31bcab68292910f7f806bc274505541d2da))
* **editorial:** block content core with sanitized renderer, workflow, revisions, media pipeline and scheduled publisher ([6112d9d](https://github.com/kiongahq/trace-proxy/commit/6112d9d8a1a9bcbb928fe7f87f5deea39f9b879d))
* establish runnable MLOps platform foundation ([70f66b9](https://github.com/kiongahq/trace-proxy/commit/70f66b941c2b9aad5d84dd4793c99172550a6795))
* **operator:** run leader-elected Kubernetes reconciliation manager ([8dd153b](https://github.com/kiongahq/trace-proxy/commit/8dd153bb781ffe53b867cedb224a678c7fe1c7a3))
* **operator:** run leader-elected Kubernetes reconciliation manager ([a1399ff](https://github.com/kiongahq/trace-proxy/commit/a1399ff60e0132d153a08318587a4a9ce7241799))
* **release:** add reviewed versions and consistent publication ([c1ed65a](https://github.com/kiongahq/trace-proxy/commit/c1ed65a9bce71d012abeab63f881e755487d5042))
* **release:** standardize versions, changelogs and publication ([2e6585c](https://github.com/kiongahq/trace-proxy/commit/2e6585c9c71cde2c66bf47698c9fd013712c3ea9))
* rename mlaiops to kionga ([#43](https://github.com/kiongahq/trace-proxy/issues/43)) ([2edcc2b](https://github.com/kiongahq/trace-proxy/commit/2edcc2ba14f6246f3e2c25b405d356c4214a3efa))
* **runtime:** add feature storage tracing metrics and CLI services ([0b61eb9](https://github.com/kiongahq/trace-proxy/commit/0b61eb9e8b3375f4da6a1241adecdcf12003ef12))
* **security:** add OIDC verification and role-based access control ([de512ce](https://github.com/kiongahq/trace-proxy/commit/de512cecea9782186c3fa1e9b626f1cf90f692f0))
* **storage:** add PostgreSQL repository and transactional outbox ([6ded5df](https://github.com/kiongahq/trace-proxy/commit/6ded5df3b2a7c6c5a2bcd654720f2c30a28a60c6))


### Fixes

* P1 trace-proxy streaming and credential fixes ([#1](https://github.com/kiongahq/trace-proxy/issues/1)-[#3](https://github.com/kiongahq/trace-proxy/issues/3)) ([#40](https://github.com/kiongahq/trace-proxy/issues/40)) ([a0b8034](https://github.com/kiongahq/trace-proxy/commit/a0b803484dabed7f18d7a0c148b7c87db82d1e5d))
* **proxy:** route hosted upstream requests correctly ([0d9d9ff](https://github.com/kiongahq/trace-proxy/commit/0d9d9ffdc9ae962106cc20b1ad292d518569b7ff))
* **release:** start unreleased components at version 0.1.0 ([42705df](https://github.com/kiongahq/trace-proxy/commit/42705df875f29e1333edc25825367dac22c39c73))
* **security:** Go 1.26.10 for crypto/x509 GO-2026-6809 ([74453e0](https://github.com/kiongahq/trace-proxy/commit/74453e09de16ef2206ba610e5f623737aee6301c))
* **security:** Go 1.26.11 for net/http GO-2026-6859 ([#44](https://github.com/kiongahq/trace-proxy/issues/44)) ([bf90f1d](https://github.com/kiongahq/trace-proxy/commit/bf90f1ddd2cba291960cd6f8de251c9dd9d2f28e))
* **security:** golang.org/x/net v0.60.0 and Go 1.26.9 ([ca4f43d](https://github.com/kiongahq/trace-proxy/commit/ca4f43d8628bd46de2e7edf6492dffc45d39b08e))
* **workspace:** make local IDE launch through current gateway ([d348ce4](https://github.com/kiongahq/trace-proxy/commit/d348ce4b106f7b6395c256210d4245087c1f3c8c))


### Documentation

* product README, CONTRIBUTING and ARCHITECTURE ([6b6b1e4](https://github.com/kiongahq/trace-proxy/commit/6b6b1e44e09500618f202dc96020f5fcfda407bf))
* **release:** align contributor publication instructions ([c893ca8](https://github.com/kiongahq/trace-proxy/commit/c893ca82cb53d26faa3804f48c4d34098761bba8))
* **security:** clarify sole-maintainer release review ([657d1d4](https://github.com/kiongahq/trace-proxy/commit/657d1d4eb22f550a6f784b38ddb929bb64deb917))


### Automation

* add pressure-test lane (full-stack + load + concurrency + vuln scan) ([59fc50c](https://github.com/kiongahq/trace-proxy/commit/59fc50c6f7048f344f5389ef989868ec4e55b5ed))


### Maintenance

* **editorial:** vendor Editor.js and highlight.js with verified integrity manifest ([908d0aa](https://github.com/kiongahq/trace-proxy/commit/908d0aaa6052f0a0dfd5a7b3b611ddcf18454b1e))
* ignore local tooling and secret files ([085a559](https://github.com/kiongahq/trace-proxy/commit/085a559a5ffb2ca74abb965c1311074ddd9f4739))
* license repository under MIT ([83389c3](https://github.com/kiongahq/trace-proxy/commit/83389c39a6cb6cb78842e32b50f86a197d5bca5d))
* **migrate:** move to the ml-ai-ops organization ([95899fd](https://github.com/kiongahq/trace-proxy/commit/95899fd96915ed002eace28ae2e77b9a9de53b94))
* move to the kiongahq org; Go module github.com/kiongahq/platform, ghcr.io/kiongahq images, Kionga product name ([2d78298](https://github.com/kiongahq/trace-proxy/commit/2d782987532c893c21061eddc0ba1c39e8e007c9))
* standalone repo: own Go module, Makefile, CI and image publishing ([9aefd7a](https://github.com/kiongahq/trace-proxy/commit/9aefd7aeae61edc12af99b44517805e1be14aae0))

## Changelog

Reviewed release pull requests add versioned changes here.
Existing tags and releases remain available in [GitHub](https://github.com/kiongahq/trace-proxy/releases).
