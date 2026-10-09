IMAGE ?= ghcr.io/kiongahq/trace-proxy:dev

.PHONY: test build image verify
test:
	go vet ./...
	go test -race ./...

build:
	go build -o bin/trace-proxy ./cmd/trace-proxy

image:
	docker build -t $(IMAGE) .

verify:
	@test -z "$$(gofmt -l .)" || { gofmt -l .; exit 1; }
	$(MAKE) test build
