GO ?= go
BIN := bin/hadoop-cli
PKG := ./...

.PHONY: all build test vet fmt lint tidy clean

all: fmt vet test build

build:
	$(GO) build -o $(BIN) .

test:
	$(GO) test $(PKG) -race

vet:
	$(GO) vet $(PKG)

fmt:
	$(GO) fmt $(PKG)
	@test -z "$$(gofmt -l .)" || (echo 'gofmt diff found'; gofmt -l .; exit 1)

tidy:
	$(GO) mod tidy

lint:
	$(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.1.6 run

clean:
	rm -rf bin dist
