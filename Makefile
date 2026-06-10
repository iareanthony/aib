BINARY := aib
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"
GOFLAGS := -trimpath

.PHONY: build run test test-verbose clean fmt lint docker

build:
	go build $(GOFLAGS) $(LDFLAGS) -o bin/$(BINARY) ./cmd/aib

run: build
	./bin/$(BINARY)

# Mirrors the CI test invocation (race detector + timeout) so local runs
# catch what CI catches.
test:
	go test -timeout 60s -race -count=1 ./...

test-verbose:
	go test -timeout 60s -race -count=1 -v ./...

clean:
	rm -rf bin/ data/

fmt:
	go fmt ./...
	goimports -w .

lint:
	golangci-lint run ./...

docker:
	docker build -t aib:$(VERSION) -f deploy/Dockerfile .

docker-compose:
	docker compose -f deploy/docker-compose.yml up --build
